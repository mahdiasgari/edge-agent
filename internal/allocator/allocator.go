package allocator

import "context"

type Result struct {
	Address string
	SNI     string
	RouteID string
}

type SNIAllocator interface {
	Allocate(
		ctx context.Context,
		domain string,
	) (Result, error)

	Release(
		ctx context.Context,
		domain string,
		result Result,
	) error
}

type RouteAllocator interface {
	Allocate(
		ctx context.Context,
		domain string,
	) (Result, error)

	Release(
		ctx context.Context,
		domain string,
		result Result,
	) error
}
