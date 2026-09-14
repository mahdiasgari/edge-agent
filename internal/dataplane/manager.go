package dataplane

import (
	"context"
	"fmt"
	"sync"

	"github.com/wraplink/edge-agent/internal/client"
)

type Manager struct {
	mu sync.Mutex

	nft *NFT

	active map[string]client.Lease
}

func NewManager() (*Manager, error) {
	nft, err := NewNFT()

	if err != nil {
		return nil, err
	}

	return &Manager{
		nft:    nft,
		active: make(map[string]client.Lease),
	}, nil
}

func (m *Manager) Apply(
	ctx context.Context,
	lease client.Lease,
) error {
	if lease.Type != "route" {
		return nil
	}

	if lease.Address == "" {
		return fmt.Errorf(
			"lease %s has no edge address",
			lease.ID,
		)
	}

	if lease.DestinationIP == "" {
		return fmt.Errorf(
			"lease %s has no destination IP",
			lease.ID,
		)
	}

	if lease.Protocol != "tcp" &&
		lease.Protocol != "udp" {
		return fmt.Errorf(
			"lease %s has invalid protocol %q",
			lease.ID,
			lease.Protocol,
		)
	}

	if lease.DestinationPort == 0 {
		return fmt.Errorf(
			"lease %s has no destination port",
			lease.ID,
		)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.active[lease.ID]; exists {
		return nil
	}

	if err := m.nft.AddRoute(
		ctx,
		lease,
	); err != nil {
		return fmt.Errorf(
			"add nft route: %w",
			err,
		)
	}

	m.active[lease.ID] = lease

	return nil
}

func (m *Manager) Remove(
	ctx context.Context,
	leaseID string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lease, ok := m.active[leaseID]

	if !ok {
		return nil
	}

	if err := m.nft.RemoveRoute(
		ctx,
		lease,
	); err != nil {
		return fmt.Errorf(
			"remove nft route: %w",
			err,
		)
	}

	delete(m.active, leaseID)

	return nil
}
