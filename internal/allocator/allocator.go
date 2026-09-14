package allocator

import "context"

type Result struct {
	Address string

	SNI string

	RouteID string

	DestinationIP string

	Protocol string

	DestinationPortStart uint16
	DestinationPortEnd   uint16
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
		destinationIP string,
		protocol string,
		destinationPortStart uint16,
		destinationPortEnd uint16,
	) (Result, error)

	Release(
		ctx context.Context,
		domain string,
		result Result,
	) error
}
