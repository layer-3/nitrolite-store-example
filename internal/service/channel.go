package service

import (
	"context"
	"fmt"

	"github.com/layer-3/nitrolite/pkg/core"
)

// ChannelService exposes read-only channel state operations.
type ChannelService struct {
	provider clientProvider
}

// NewChannelService constructs a ChannelService.
func NewChannelService(provider clientProvider) *ChannelService {
	return &ChannelService{provider: provider}
}

// GetHomeChannel retrieves the current home channel for an asset.
func (s *ChannelService) GetHomeChannel(ctx context.Context, asset string) (*core.Channel, error) {
	client, wallet, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	channel, err := client.GetHomeChannel(ctx, wallet, asset)
	if err != nil {
		return nil, fmt.Errorf("failed to get home channel: %w", err)
	}
	return channel, nil
}

// GetLatestState retrieves the latest state for an asset.
func (s *ChannelService) GetLatestState(ctx context.Context, asset string, onlySigned bool) (*core.State, error) {
	client, wallet, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	state, err := client.GetLatestState(ctx, wallet, asset, onlySigned)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest state: %w", err)
	}
	return state, nil
}
