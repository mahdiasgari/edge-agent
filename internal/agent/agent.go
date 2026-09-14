package agent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/wraplink/edge-agent/internal/allocator"
	"github.com/wraplink/edge-agent/internal/client"
)

const (
	TypeSNI   = "sni"
	TypeRoute = "route"
)

type Agent struct {
	id       string
	interval time.Duration

	sniCapacity   int
	routeCapacity int

	dnsClient *client.Client

	sniAllocator   allocator.SNIAllocator
	routeAllocator allocator.RouteAllocator

	mu     sync.RWMutex
	active map[string]client.Lease
}

func New(
	id string,
	interval time.Duration,
	sniCapacity int,
	routeCapacity int,
	dnsClient *client.Client,
	sniAllocator allocator.SNIAllocator,
	routeAllocator allocator.RouteAllocator,
) *Agent {
	return &Agent{
		id:            id,
		interval:      interval,
		sniCapacity:   sniCapacity,
		routeCapacity: routeCapacity,

		dnsClient: dnsClient,

		sniAllocator:   sniAllocator,
		routeAllocator: routeAllocator,

		active: make(map[string]client.Lease),
	}
}

func (a *Agent) Run(ctx context.Context) error {
	log.Printf(
		"agent: starting id=%s interval=%s",
		a.id,
		a.interval,
	)

	if err := a.register(ctx); err != nil {
		return fmt.Errorf(
			"register agent: %w",
			err,
		)
	}

	// Do the first poll immediately.
	if err := a.cycle(ctx); err != nil {
		log.Printf(
			"agent: initial cycle failed: %v",
			err,
		)
	}

	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
			if err := a.cycle(ctx); err != nil {
				log.Printf(
					"agent: cycle failed: %v",
					err,
				)
			}
		}
	}
}

func (a *Agent) register(
	ctx context.Context,
) error {
	return a.dnsClient.Register(
		ctx,
		a.id,
		a.sniCapacity,
		a.routeCapacity,
	)
}

func (a *Agent) cycle(
	ctx context.Context,
) error {
	// Renew leases first.
	if err := a.sendHeartbeat(ctx); err != nil {
		log.Printf(
			"agent: heartbeat failed: %v",
			err,
		)
	}

	// Ask dns-control for new work.
	max := a.availableSlots()

	if max <= 0 {
		return nil
	}

	response, err := a.dnsClient.Poll(
		ctx,
		a.id,
		max,
	)
	if err != nil {
		return fmt.Errorf(
			"poll dns-control: %w",
			err,
		)
	}

	if len(response.Leases) == 0 {
		return nil
	}

	log.Printf(
		"agent: received %d lease(s)",
		len(response.Leases),
	)

	for _, lease := range response.Leases {
		if err := a.processLease(
			ctx,
			lease,
		); err != nil {
			log.Printf(
				"agent: process lease id=%s domain=%s type=%s: %v",
				lease.ID,
				lease.Domain,
				lease.Type,
				err,
			)

			// Tell dns-control that allocation failed.
			if reportErr := a.reportFailure(
				ctx,
				lease,
			); reportErr != nil {
				log.Printf(
					"agent: report failure lease=%s: %v",
					lease.ID,
					reportErr,
				)
			}
		}
	}

	return nil
}

func (a *Agent) processLease(
	ctx context.Context,
	lease client.Lease,
) error {
	switch lease.Type {
	case TypeSNI:
		return a.processSNILease(
			ctx,
			lease,
		)

	case TypeRoute:
		return a.processRouteLease(
			ctx,
			lease,
		)

	default:
		return fmt.Errorf(
			"unsupported lease type %q",
			lease.Type,
		)
	}
}

func (a *Agent) processSNILease(
	ctx context.Context,
	lease client.Lease,
) error {
	// Protect against accidentally exceeding our configured capacity.
	if a.countActive(TypeSNI) >= a.sniCapacity {
		return fmt.Errorf(
			"SNI capacity exhausted",
		)
	}

	result, err := a.sniAllocator.Allocate(
		ctx,
		lease.Domain,
	)
	if err != nil {
		return fmt.Errorf(
			"allocate SNI: %w",
			err,
		)
	}

	report := client.ReportRequest{
		AgentID: a.id,
		LeaseID: lease.ID,
		Success: true,
		Address: result.Address,
		SNI:     result.SNI,
	}

	if err := a.dnsClient.Report(
		ctx,
		report,
	); err != nil {
		// DNS-control did not accept the allocation.
		// Release the resource we just allocated.
		if releaseErr := a.sniAllocator.Release(
			ctx,
			lease.Domain,
			result,
		); releaseErr != nil {
			log.Printf(
				"agent: release SNI after report failure domain=%s: %v",
				lease.Domain,
				releaseErr,
			)
		}

		return fmt.Errorf(
			"report SNI allocation: %w",
			err,
		)
	}

	a.mu.Lock()
	a.active[lease.ID] = lease
	a.mu.Unlock()

	log.Printf(
		"agent: SNI active domain=%s address=%s sni=%s lease=%s",
		lease.Domain,
		result.Address,
		result.SNI,
		lease.ID,
	)

	return nil
}

func (a *Agent) processRouteLease(
	ctx context.Context,
	lease client.Lease,
) error {
	if a.countActive(TypeRoute) >= a.routeCapacity {
		return fmt.Errorf(
			"route capacity exhausted",
		)
	}

	result, err := a.routeAllocator.Allocate(
		ctx,
		lease.Domain,
	)
	if err != nil {
		return fmt.Errorf(
			"allocate route: %w",
			err,
		)
	}

	report := client.ReportRequest{
		AgentID: a.id,
		LeaseID: lease.ID,
		Success: true,
		Address: result.Address,
		RouteID: result.RouteID,
	}

	if err := a.dnsClient.Report(
		ctx,
		report,
	); err != nil {
		if releaseErr := a.routeAllocator.Release(
			ctx,
			lease.Domain,
			result,
		); releaseErr != nil {
			log.Printf(
				"agent: release route after report failure domain=%s: %v",
				lease.Domain,
				releaseErr,
			)
		}

		return fmt.Errorf(
			"report route allocation: %w",
			err,
		)
	}

	a.mu.Lock()
	a.active[lease.ID] = lease
	a.mu.Unlock()

	log.Printf(
		"agent: route active domain=%s address=%s route_id=%s lease=%s",
		lease.Domain,
		result.Address,
		result.RouteID,
		lease.ID,
	)

	return nil
}

func (a *Agent) reportFailure(
	ctx context.Context,
	lease client.Lease,
) error {
	return a.dnsClient.Report(
		ctx,
		client.ReportRequest{
			AgentID: a.id,
			LeaseID: lease.ID,
			Success: false,
		},
	)
}

func (a *Agent) sendHeartbeat(
	ctx context.Context,
) error {
	a.mu.RLock()

	leaseIDs := make([]string, 0, len(a.active))

	for id := range a.active {
		leaseIDs = append(
			leaseIDs,
			id,
		)
	}

	a.mu.RUnlock()

	if len(leaseIDs) == 0 {
		return nil
	}

	return a.dnsClient.Heartbeat(
		ctx,
		a.id,
		leaseIDs,
	)
}

func (a *Agent) availableSlots() int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	sniUsed := 0
	routeUsed := 0

	for _, lease := range a.active {
		switch lease.Type {
		case TypeSNI:
			sniUsed++

		case TypeRoute:
			routeUsed++
		}
	}

	sniAvailable := a.sniCapacity - sniUsed
	routeAvailable := a.routeCapacity - routeUsed

	if sniAvailable < 0 {
		sniAvailable = 0
	}

	if routeAvailable < 0 {
		routeAvailable = 0
	}

	return sniAvailable + routeAvailable
}

func (a *Agent) countActive(
	leaseType string,
) int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	count := 0

	for _, lease := range a.active {
		if lease.Type == leaseType {
			count++
		}
	}

	return count
}
