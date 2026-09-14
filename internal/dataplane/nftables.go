package dataplane

import (
	"context"

	"github.com/wraplink/edge-agent/internal/client"
)

type NFT struct {
}

func NewNFT() (*NFT, error) {
	return &NFT{}, nil
}

func (n *NFT) AddRoute(
	ctx context.Context,
	lease client.Lease,
) error {
	return nil
}

func (n *NFT) RemoveRoute(
	ctx context.Context,
	lease client.Lease,
) error {
	return nil
}
