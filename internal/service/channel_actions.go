package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/layer-3/nitrolite/pkg/core"
	"github.com/shopspring/decimal"
)

// CloseChannelRequest contains close-channel inputs.
type CloseChannelRequest struct {
	Asset string
}

// ChallengeRequest contains challenge inputs.
type ChallengeRequest struct {
	Asset string
}

// ChallengeResult captures challenge tx hash.
type ChallengeResult struct {
	TxHash string
}

// CloseChannel closes the home channel for an asset.
func (s *ChannelService) CloseChannel(ctx context.Context, req CloseChannelRequest) (*core.State, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}
	asset := strings.TrimSpace(strings.ToLower(req.Asset))
	if asset == "" {
		return nil, invalidf("asset is required")
	}
	state, err := client.CloseHomeChannel(ctx, asset)
	if err != nil {
		return nil, fmt.Errorf("failed to close home channel: %w", err)
	}
	return state, nil
}

// ChallengeLatestState challenges the latest signed state for an asset.
func (s *ChannelService) ChallengeLatestState(ctx context.Context, req ChallengeRequest) (*ChallengeResult, error) {
	client, wallet, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}
	asset := strings.TrimSpace(strings.ToLower(req.Asset))
	if asset == "" {
		return nil, invalidf("asset is required")
	}
	state, err := client.GetLatestState(ctx, wallet, asset, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest signed state: %w", err)
	}
	txHash, err := client.Challenge(ctx, *state)
	if err != nil {
		return nil, fmt.Errorf("failed to challenge state: %w", err)
	}
	return &ChallengeResult{TxHash: txHash}, nil
}

// MutationService exposes phase-1/3 channel mutation operations.
type MutationService struct {
	provider        clientProvider
	homeBlockchains map[string]uint64
}

// NewMutationService constructs a MutationService.
func NewMutationService(provider clientProvider, homeBlockchains map[string]uint64) *MutationService {
	normalized := make(map[string]uint64, len(homeBlockchains))
	for asset, chainID := range homeBlockchains {
		normalized[strings.ToLower(strings.TrimSpace(asset))] = chainID
	}
	return &MutationService{
		provider:        provider,
		homeBlockchains: normalized,
	}
}

// ApproveToken approves token spending for the demo signer.
func (s *MutationService) ApproveToken(ctx context.Context, chainID uint64, asset string, amount decimal.Decimal) (string, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return "", err
	}
	normalizedAsset, err := normalizeAsset(asset)
	if err != nil {
		return "", err
	}
	if chainID == 0 {
		return "", invalidf("blockchain_id must be positive")
	}
	if err := s.validateHomeBlockchain(normalizedAsset, chainID); err != nil {
		return "", err
	}
	if !amount.IsPositive() {
		return "", invalidf("amount must be positive")
	}

	txHash, err := client.ApproveToken(ctx, chainID, normalizedAsset, amount)
	if err != nil {
		return "", fmt.Errorf("failed to approve token: %w", err)
	}
	return txHash, nil
}

// Deposit submits a home deposit.
func (s *MutationService) Deposit(ctx context.Context, chainID uint64, asset string, amount decimal.Decimal) (*core.State, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}
	normalizedAsset, err := normalizeAsset(asset)
	if err != nil {
		return nil, err
	}
	if chainID == 0 {
		return nil, invalidf("blockchain_id must be positive")
	}
	if err := s.validateHomeBlockchain(normalizedAsset, chainID); err != nil {
		return nil, err
	}
	if !amount.IsPositive() {
		return nil, invalidf("amount must be positive")
	}

	state, err := client.Deposit(ctx, chainID, normalizedAsset, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to deposit: %w", err)
	}
	return state, nil
}

// Withdraw submits a home withdrawal.
func (s *MutationService) Withdraw(ctx context.Context, chainID uint64, asset string, amount decimal.Decimal) (*core.State, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}
	normalizedAsset, err := normalizeAsset(asset)
	if err != nil {
		return nil, err
	}
	if chainID == 0 {
		return nil, invalidf("blockchain_id must be positive")
	}
	if err := s.validateHomeBlockchain(normalizedAsset, chainID); err != nil {
		return nil, err
	}
	if !amount.IsPositive() {
		return nil, invalidf("amount must be positive")
	}

	state, err := client.Withdraw(ctx, chainID, normalizedAsset, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to withdraw: %w", err)
	}
	return state, nil
}

// Transfer submits a transfer.
func (s *MutationService) Transfer(ctx context.Context, recipient string, asset string, amount decimal.Decimal) (*core.State, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}
	normalizedAsset, err := normalizeAsset(asset)
	if err != nil {
		return nil, err
	}
	normalizedRecipient := strings.TrimSpace(recipient)
	if normalizedRecipient == "" {
		return nil, invalidf("recipient is required")
	}
	if !common.IsHexAddress(normalizedRecipient) {
		return nil, invalidf("invalid recipient")
	}
	if !amount.IsPositive() {
		return nil, invalidf("amount must be positive")
	}

	state, err := client.Transfer(ctx, normalizedRecipient, normalizedAsset, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to transfer: %w", err)
	}
	return state, nil
}

// Checkpoint submits a checkpoint.
func (s *MutationService) Checkpoint(ctx context.Context, asset string) (string, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return "", err
	}
	normalizedAsset, err := normalizeAsset(asset)
	if err != nil {
		return "", err
	}

	txHash, err := client.Checkpoint(ctx, normalizedAsset)
	if err != nil {
		return "", fmt.Errorf("failed to checkpoint: %w", err)
	}
	return txHash, nil
}

func normalizeAsset(asset string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(asset))
	if normalized == "" {
		return "", invalidf("asset is required")
	}
	return normalized, nil
}

func (s *MutationService) validateHomeBlockchain(asset string, chainID uint64) error {
	expectedChainID, ok := s.homeBlockchains[asset]
	if !ok {
		return invalidf("asset %s is not configured in HOME_BLOCKCHAINS", asset)
	}
	if expectedChainID != chainID {
		return invalidf("blockchain_id must match configured home chain %d for asset %s", expectedChainID, asset)
	}
	return nil
}
