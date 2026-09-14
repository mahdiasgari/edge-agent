package allocator

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
)

type Route struct {
	mu        sync.Mutex
	addresses []string
	assigned  map[string]Result
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
) (Result, error) {
	domain = normalize(domain)

	if domain == "" {
		return Result{}, fmt.Errorf(
			"domain is empty",
		)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if result, ok := a.assigned[domain]; ok {
		return result, nil
	}

	if len(a.addresses) == 0 {
		return Result{}, fmt.Errorf(
			"no edge addresses available",
		)
	}

	index := hash(domain) % uint32(len(a.addresses))

	address := a.addresses[index]

	result := Result{
		Address: address,
		RouteID: "route-" + hashString(domain),
	}

	a.assigned[domain] = result

	return result, nil
}

func (a *Route) Release(
	_ context.Context,
	domain string,
	_ Result,
) error {
	domain = normalize(domain)

	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.assigned, domain)

	return nil
}

func hashString(value string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))

	return fmt.Sprintf(
		"%08x",
		h.Sum32(),
	)
}
