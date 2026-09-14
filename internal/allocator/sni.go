package allocator

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type SNI struct {
	mu        sync.Mutex
	addresses []string
	assigned  map[string]string
}

func NewSNI(addresses []string) *SNI {
	cleaned := make([]string, 0, len(addresses))

	for _, address := range addresses {
		address = strings.TrimSpace(address)

		if address == "" {
			continue
		}

		cleaned = append(cleaned, address)
	}

	return &SNI{
		addresses: cleaned,
		assigned:  make(map[string]string),
	}
}

func (a *SNI) Allocate(
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

	if address, ok := a.assigned[domain]; ok {
		return Result{
			Address: address,
			SNI:     domain,
		}, nil
	}

	if len(a.addresses) == 0 {
		return Result{}, fmt.Errorf(
			"no edge addresses available",
		)
	}

	index := hash(domain) % uint32(len(a.addresses))
	address := a.addresses[index]

	a.assigned[domain] = address

	return Result{
		Address: address,
		SNI:     domain,
	}, nil
}

func (a *SNI) Release(
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
