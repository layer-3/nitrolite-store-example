package service

import (
	"context"
	"fmt"

	"github.com/layer-3/nitrolite/pkg/core"
)

// NodeService exposes read-only node metadata operations.
type NodeService struct {
	provider clientProvider
}

// NewNodeService constructs a NodeService.
func NewNodeService(provider clientProvider) *NodeService {
	return &NodeService{provider: provider}
}

// GetConfig retrieves clearnode configuration.
func (s *NodeService) GetConfig(ctx context.Context) (*core.NodeConfig, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	cfg, err := client.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get node config: %w", err)
	}
	return cfg, nil
}

// GetBlockchains retrieves supported blockchains.
func (s *NodeService) GetBlockchains(ctx context.Context) ([]core.Blockchain, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	blockchains, err := client.GetBlockchains(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get blockchains: %w", err)
	}
	return blockchains, nil
}

// GetAssets retrieves supported assets.
func (s *NodeService) GetAssets(ctx context.Context, blockchainID *uint64) ([]core.Asset, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	assets, err := client.GetAssets(ctx, blockchainID)
	if err != nil {
		return nil, fmt.Errorf("failed to get assets: %w", err)
	}
	return assets, nil
}
