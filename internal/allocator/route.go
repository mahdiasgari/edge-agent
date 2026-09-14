package allocator

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
)

type Route struct {
	mu sync.Mutex

	addresses []string

	assigned map[string]Result
}

func NewRoute(addresses []string) *Route {
	cleaned := make([]string, 0, len(addresses))

	for _, address := range addresses {
		address = strings.TrimSpace(address)

		if address == "" {
			continue
		}

		cleaned = append(cleaned, address)
	}

	return &Route{
		addresses: cleaned,
		assigned:  make(map[string]Result),
	}
}

func (a *Route) Allocate(
	_ context.Context,
	domain string,
	destinationIP string,
	protocol string,
	destinationPort uint16,
	sourcePort uint16,
) (Result, error) {
	domain = normalize(domain)
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	destinationIP = strings.TrimSpace(destinationIP)

	if domain == "" {
		return Result{}, fmt.Errorf("domain is empty")
	}

	if net.ParseIP(destinationIP) == nil {
		return Result{}, fmt.Errorf(
			"invalid destination IP %q",
			destinationIP,
		)
	}

	switch protocol {
	case "tcp", "udp":
	default:
		return Result{}, fmt.Errorf(
			"unsupported protocol %q",
			protocol,
		)
	}

	if destinationPort == 0 {
		return Result{}, fmt.Errorf(
			"destination port is required",
		)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	key := routeKey(
		domain,
		destinationIP,
		protocol,
		destinationPort,
		sourcePort,
	)

	if result, ok := a.assigned[key]; ok {
		return result, nil
	}

	if len(a.addresses) == 0 {
		return Result{}, fmt.Errorf(
			"no edge addresses available",
		)
	}

	index := hash(key) % uint32(len(a.addresses))

	address := a.addresses[index]

	result := Result{
		Address:         address,
		RouteID:         "route-" + hashString(key),
		DestinationIP:   destinationIP,
		Protocol:        protocol,
		DestinationPort: destinationPort,
		SourcePort:      sourcePort,
	}

	a.assigned[key] = result

	return result, nil
}

func (a *Route) Release(
	_ context.Context,
	domain string,
	result Result,
) error {
	domain = normalize(domain)

	key := routeKey(
		domain,
		result.DestinationIP,
		result.Protocol,
		result.DestinationPort,
		result.SourcePort,
	)

	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.assigned, key)

	return nil
}

func routeKey(
	domain string,
	destinationIP string,
	protocol string,
	destinationPort uint16,
	sourcePort uint16,
) string {
	return fmt.Sprintf(
		"%s|%s|%s|%d|%d",
		domain,
		destinationIP,
		protocol,
		destinationPort,
		sourcePort,
	)
}
