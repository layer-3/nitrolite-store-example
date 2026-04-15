package service

import (
	"context"
	"sort"
	"strings"

	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
	sdk "github.com/layer-3/nitrolite/sdk/go"
)

// DemoGuidance describes the next recommended action for a guided flow.
type DemoGuidance struct {
	NextAction  string
	Headline    string
	Description string
	SyncPending bool
}

// DemoOverview captures the landing-page view of the demo state.
type DemoOverview struct {
	Health          nitrolite.Health
	WalletAddress   string
	HomeBlockchains map[string]uint64
	SelectedAsset   string
	Assets          []core.Asset
	Balances        []core.BalanceEntry
	Channel         *core.Channel
	LatestState     *core.State
	LatestActivity  *core.Transaction
	Apps            []app.AppInfoV1
	Sessions        []app.AppSessionInfoV1
	ChannelGuidance DemoGuidance
	AppGuidance     DemoGuidance
}

// DemoService provides aggregated, UI-oriented reads for the guided surface.
type DemoService struct {
	provider        clientProvider
	health          func() nitrolite.Health
	homeBlockchains map[string]uint64
}

// NewDemoService constructs a DemoService.
func NewDemoService(provider clientProvider, health func() nitrolite.Health, homeBlockchains map[string]uint64) *DemoService {
	normalized := make(map[string]uint64, len(homeBlockchains))
	for asset, chainID := range homeBlockchains {
		normalized[strings.ToLower(strings.TrimSpace(asset))] = chainID
	}
	return &DemoService{
		provider:        provider,
		health:          health,
		homeBlockchains: normalized,
	}
}

// GetOverview aggregates the current demo state for the guided landing page.
func (s *DemoService) GetOverview(ctx context.Context, requestedAsset string) (*DemoOverview, error) {
	client, wallet, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	assets, err := client.GetAssets(ctx, nil)
	if err != nil {
		return nil, err
	}
	balances, err := client.GetBalances(ctx, wallet)
	if err != nil {
		return nil, err
	}
	selectedAsset := chooseSelectedAsset(requestedAsset, balances, assets, s.homeBlockchains)

	var channel *core.Channel
	if selectedAsset != "" {
		channel, _ = client.GetHomeChannel(ctx, wallet, selectedAsset)
	}

	var latestState *core.State
	if selectedAsset != "" {
		latestState, _ = client.GetLatestState(ctx, wallet, selectedAsset, true)
	}

	var latestActivity *core.Transaction
	offset := uint32(0)
	limit := uint32(5)
	if transactions, _, err := client.GetTransactions(ctx, wallet, &sdk.GetTransactionsOptions{
		Pagination: &core.PaginationParams{Offset: &offset, Limit: &limit},
	}); err == nil && len(transactions) > 0 {
		latestActivity = &transactions[0]
	}

	apps, _, _ := client.GetApps(ctx, &sdk.GetAppsOptions{
		Pagination: &core.PaginationParams{Offset: &offset, Limit: &limit},
	})
	sessions, _, _ := client.GetAppSessions(ctx, &sdk.GetAppSessionsOptions{
		Participant: &wallet,
		Pagination:  &core.PaginationParams{Offset: &offset, Limit: &limit},
	})

	overview := &DemoOverview{
		Health:          s.health(),
		WalletAddress:   wallet,
		HomeBlockchains: cloneHomeBlockchains(s.homeBlockchains),
		SelectedAsset:   selectedAsset,
		Assets:          assets,
		Balances:        balances,
		Channel:         channel,
		LatestState:     latestState,
		LatestActivity:  latestActivity,
		Apps:            apps,
		Sessions:        sessions,
	}
	overview.ChannelGuidance = buildChannelGuidance(overview)
	overview.AppGuidance = buildAppGuidance(overview)
	return overview, nil
}

func chooseSelectedAsset(requested string, balances []core.BalanceEntry, assets []core.Asset, homeBlockchains map[string]uint64) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested != "" {
		for _, asset := range assets {
			if strings.EqualFold(asset.Symbol, requested) {
				return strings.ToLower(asset.Symbol)
			}
		}
	}

	for _, balance := range balances {
		if strings.TrimSpace(balance.Asset) != "" {
			return strings.ToLower(balance.Asset)
		}
	}

	homeAssets := make([]string, 0, len(homeBlockchains))
	for asset := range homeBlockchains {
		homeAssets = append(homeAssets, asset)
	}
	sort.Strings(homeAssets)
	if len(homeAssets) > 0 {
		return homeAssets[0]
	}

	available := make([]string, 0, len(assets))
	for _, asset := range assets {
		if strings.TrimSpace(asset.Symbol) != "" {
			available = append(available, strings.ToLower(asset.Symbol))
		}
	}
	sort.Strings(available)
	if len(available) > 0 {
		return available[0]
	}

	return ""
}

func cloneHomeBlockchains(values map[string]uint64) map[string]uint64 {
	out := make(map[string]uint64, len(values))
	for asset, chainID := range values {
		out[asset] = chainID
	}
	return out
}

func buildChannelGuidance(overview *DemoOverview) DemoGuidance {
	if overview.SelectedAsset == "" {
		return DemoGuidance{
			NextAction:  "review_supported_assets",
			Headline:    "No supported asset selected",
			Description: "Pick a supported asset to inspect the live channel flow.",
		}
	}

	if overview.Channel != nil && overview.LatestState != nil && overview.LatestState.Version > overview.Channel.StateVersion {
		if isCheckpointableTransition(overview.LatestState.Transition.Type) {
			return DemoGuidance{
				NextAction:  "checkpoint",
				Headline:    "Latest signed channel transition is ready to checkpoint",
				Description: "Checkpoint the pending home deposit or withdrawal before starting the next channel transition.",
				SyncPending: true,
			}
		}
		return DemoGuidance{
			NextAction:  "wait_for_sync",
			Headline:    "Latest signed state is ahead of clearnode channel state",
			Description: "Waiting for clearnode sync before the next channel transition. The pending signed state comes from app-session activity, so this guided flow should wait instead of checkpointing manually.",
			SyncPending: true,
		}
	}

	balance := balanceForAsset(overview.Balances, overview.SelectedAsset)
	if balance == "0" {
		return DemoGuidance{
			NextAction:  "approve_and_deposit",
			Headline:    "Fund the home channel",
			Description: "Approve the asset and deposit into the home channel to start the live channel flow.",
		}
	}

	return DemoGuidance{
		NextAction:  "transfer_or_withdraw",
		Headline:    "Channel is synced and ready",
		Description: "Run a deposit, transfer, withdraw, or app-session flow using the live backend signer.",
	}
}

func buildAppGuidance(overview *DemoOverview) DemoGuidance {
	for _, session := range overview.Sessions {
		if !session.IsClosed {
			return DemoGuidance{
				NextAction:  "operate_or_close_session",
				Headline:    "Open app session available",
				Description: "Continue the single-signer app flow with deposit, operate, or close.",
			}
		}
	}

	for _, appInfo := range overview.Apps {
		if strings.EqualFold(appInfo.App.ID, "default") {
			return DemoGuidance{
				NextAction:  "create_session",
				Headline:    "Ready to create a demo app session",
				Description: "Use the default app to create a single-signer session, then deposit, operate, and close it.",
			}
		}
	}

	return DemoGuidance{
		NextAction:  "review_apps",
		Headline:    "No app session ready yet",
		Description: "Review available apps first. Registering a new app may require staked YELLOW depending on node policy.",
	}
}

func balanceForAsset(balances []core.BalanceEntry, asset string) string {
	for _, balance := range balances {
		if strings.EqualFold(balance.Asset, asset) {
			return balance.Balance.String()
		}
	}
	return "0"
}

func isCheckpointableTransition(transitionType core.TransitionType) bool {
	switch transitionType {
	case core.TransitionTypeHomeDeposit, core.TransitionTypeHomeWithdrawal:
		return true
	default:
		return false
	}
}
