package httpapi

import (
	"time"

	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-go-example/internal/service"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
)

type nodeConfigResponse struct {
	NodeAddress            string               `json:"nodeAddress"`
	NodeVersion            string               `json:"nodeVersion"`
	SupportedSigValidators []string             `json:"supportedSigValidators"`
	Blockchains            []blockchainResponse `json:"blockchains"`
}

type blockchainResponse struct {
	ID                     uint64 `json:"id"`
	Name                   string `json:"name"`
	ChannelHubAddress      string `json:"channelHubAddress"`
	LockingContractAddress string `json:"lockingContractAddress"`
	BlockStep              uint64 `json:"blockStep"`
}

type assetResponse struct {
	Name                  string          `json:"name"`
	Symbol                string          `json:"symbol"`
	Decimals              uint8           `json:"decimals"`
	SuggestedBlockchainID uint64          `json:"suggestedBlockchainID"`
	Tokens                []tokenResponse `json:"tokens"`
}

type tokenResponse struct {
	Name         string `json:"name"`
	Symbol       string `json:"symbol"`
	Address      string `json:"address"`
	BlockchainID uint64 `json:"blockchainId"`
	Decimals     uint8  `json:"decimals"`
}

type balanceResponse struct {
	Asset   string `json:"asset"`
	Balance string `json:"balance"`
}

type paginationResponse struct {
	Page       uint32 `json:"page"`
	PerPage    uint32 `json:"perPage"`
	TotalCount uint32 `json:"totalCount"`
	PageCount  uint32 `json:"pageCount"`
}

type transactionResponse struct {
	ID                 string  `json:"id"`
	Asset              string  `json:"asset"`
	Type               string  `json:"type"`
	From               string  `json:"from"`
	To                 string  `json:"to"`
	SenderNewStateID   *string `json:"senderNewStateID,omitempty"`
	ReceiverNewStateID *string `json:"receiverNewStateID,omitempty"`
	Amount             string  `json:"amount"`
	Timestamp          string  `json:"timestamp"`
}

type channelResponse struct {
	ChannelID             string  `json:"channelID"`
	UserWallet            string  `json:"userWallet"`
	Asset                 string  `json:"asset"`
	Type                  string  `json:"type"`
	BlockchainID          uint64  `json:"blockchainID"`
	TokenAddress          string  `json:"tokenAddress"`
	ChallengeDuration     uint32  `json:"challengeDuration"`
	ChallengeExpiresAt    *string `json:"challengeExpiresAt,omitempty"`
	Nonce                 uint64  `json:"nonce"`
	ApprovedSigValidators string  `json:"approvedSigValidators"`
	Status                string  `json:"status"`
	StateVersion          uint64  `json:"stateVersion"`
}

type stateResponse struct {
	ID              string             `json:"id"`
	Asset           string             `json:"asset"`
	UserWallet      string             `json:"userWallet"`
	Epoch           uint64             `json:"epoch"`
	Version         uint64             `json:"version"`
	HomeChannelID   *string            `json:"homeChannelID,omitempty"`
	EscrowChannelID *string            `json:"escrowChannelID,omitempty"`
	Transition      transitionResponse `json:"transition"`
	HomeLedger      ledgerResponse     `json:"homeLedger"`
	EscrowLedger    *ledgerResponse    `json:"escrowLedger,omitempty"`
	UserSig         *string            `json:"userSig,omitempty"`
	NodeSig         *string            `json:"nodeSig,omitempty"`
}

type transitionResponse struct {
	Type      string `json:"type"`
	TxID      string `json:"txID"`
	AccountID string `json:"accountID"`
	Amount    string `json:"amount"`
}

type ledgerResponse struct {
	TokenAddress string `json:"tokenAddress"`
	BlockchainID uint64 `json:"blockchainID"`
	UserBalance  string `json:"userBalance"`
	UserNetFlow  string `json:"userNetFlow"`
	NodeBalance  string `json:"nodeBalance"`
	NodeNetFlow  string `json:"nodeNetFlow"`
}

type appInfoResponse struct {
	AppID                       string `json:"app_id"`
	OwnerWallet                 string `json:"owner_wallet"`
	Metadata                    string `json:"metadata"`
	Version                     uint64 `json:"version"`
	CreationApprovalNotRequired bool   `json:"creation_approval_not_required"`
	CreatedAt                   string `json:"created_at"`
	UpdatedAt                   string `json:"updated_at"`
}

type appParticipantResponse struct {
	WalletAddress   string `json:"wallet_address"`
	SignatureWeight uint8  `json:"signature_weight"`
}

type appDefinitionResponse struct {
	ApplicationID string                   `json:"application_id"`
	Participants  []appParticipantResponse `json:"participants"`
	Quorum        uint8                    `json:"quorum"`
	Nonce         uint64                   `json:"nonce"`
}

type appAllocationResponse struct {
	Participant string `json:"participant"`
	Asset       string `json:"asset"`
	Amount      string `json:"amount"`
}

type appSessionResponse struct {
	AppSessionID  string                   `json:"app_session_id"`
	ApplicationID string                   `json:"application_id"`
	Participants  []appParticipantResponse `json:"participants"`
	Quorum        uint8                    `json:"quorum"`
	Nonce         uint64                   `json:"nonce"`
	Status        string                   `json:"status"`
	Version       uint64                   `json:"version"`
	SessionData   string                   `json:"session_data"`
	Allocations   []appAllocationResponse  `json:"allocations"`
}

type appSessionKeyStateResponse struct {
	UserAddress    string   `json:"user_address"`
	SessionKey     string   `json:"session_key"`
	Version        uint64   `json:"version"`
	ApplicationIDs []string `json:"application_ids"`
	AppSessionIDs  []string `json:"app_session_ids"`
	ExpiresAt      string   `json:"expires_at"`
	UserSig        string   `json:"user_sig"`
}

type channelSessionKeyStateResponse struct {
	UserAddress string   `json:"user_address"`
	SessionKey  string   `json:"session_key"`
	Version     uint64   `json:"version"`
	Assets      []string `json:"assets"`
	ExpiresAt   string   `json:"expires_at"`
	UserSig     string   `json:"user_sig"`
}

type healthResponse struct {
	Connected     bool   `json:"connected"`
	Ready         bool   `json:"ready"`
	SignerAddress string `json:"signer_address"`
}

type walletResponse struct {
	Address         string            `json:"address"`
	HomeBlockchains map[string]uint64 `json:"home_blockchains"`
}

type demoGuidanceResponse struct {
	NextAction  string `json:"next_action"`
	Headline    string `json:"headline"`
	Description string `json:"description"`
	SyncPending bool   `json:"sync_pending"`
}

type demoOverviewResponse struct {
	Status          healthResponse       `json:"status"`
	Wallet          walletResponse       `json:"wallet"`
	SelectedAsset   string               `json:"selected_asset"`
	Assets          []assetResponse      `json:"assets"`
	Balances        []balanceResponse    `json:"balances"`
	Channel         *channelResponse     `json:"channel,omitempty"`
	LatestState     *stateResponse       `json:"latest_state,omitempty"`
	LatestActivity  *transactionResponse `json:"latest_activity,omitempty"`
	Apps            []appInfoResponse    `json:"apps"`
	Sessions        []appSessionResponse `json:"sessions"`
	ChannelGuidance demoGuidanceResponse `json:"channel_guidance"`
	AppGuidance     demoGuidanceResponse `json:"app_guidance"`
}

func encodeNodeConfig(cfg *core.NodeConfig) nodeConfigResponse {
	supported := make([]string, 0, len(cfg.SupportedSigValidators))
	for _, validator := range cfg.SupportedSigValidators {
		supported = append(supported, validator.String())
	}

	return nodeConfigResponse{
		NodeAddress:            cfg.NodeAddress,
		NodeVersion:            cfg.NodeVersion,
		SupportedSigValidators: supported,
		Blockchains:            encodeBlockchains(cfg.Blockchains),
	}
}

func encodeHealth(health nitrolite.Health) healthResponse {
	return healthResponse{
		Connected:     health.Connected,
		Ready:         health.Ready,
		SignerAddress: health.SignerAddress,
	}
}

func encodeBlockchains(blockchains []core.Blockchain) []blockchainResponse {
	items := make([]blockchainResponse, 0, len(blockchains))
	for _, blockchain := range blockchains {
		items = append(items, blockchainResponse{
			ID:                     blockchain.ID,
			Name:                   blockchain.Name,
			ChannelHubAddress:      blockchain.ChannelHubAddress,
			LockingContractAddress: blockchain.LockingContractAddress,
			BlockStep:              blockchain.BlockStep,
		})
	}
	return items
}

func encodeAssets(assets []core.Asset) []assetResponse {
	items := make([]assetResponse, 0, len(assets))
	for _, asset := range assets {
		tokens := make([]tokenResponse, 0, len(asset.Tokens))
		for _, token := range asset.Tokens {
			tokens = append(tokens, tokenResponse{
				Name:         token.Name,
				Symbol:       token.Symbol,
				Address:      token.Address,
				BlockchainID: token.BlockchainID,
				Decimals:     token.Decimals,
			})
		}

		items = append(items, assetResponse{
			Name:                  asset.Name,
			Symbol:                asset.Symbol,
			Decimals:              asset.Decimals,
			SuggestedBlockchainID: asset.SuggestedBlockchainID,
			Tokens:                tokens,
		})
	}
	return items
}

func encodeBalances(balances []core.BalanceEntry) []balanceResponse {
	items := make([]balanceResponse, 0, len(balances))
	for _, balance := range balances {
		items = append(items, balanceResponse{
			Asset:   balance.Asset,
			Balance: balance.Balance.String(),
		})
	}
	return items
}

func encodePagination(meta core.PaginationMetadata) paginationResponse {
	return paginationResponse{
		Page:       meta.Page,
		PerPage:    meta.PerPage,
		TotalCount: meta.TotalCount,
		PageCount:  meta.PageCount,
	}
}

func encodeTransactions(transactions []core.Transaction) []transactionResponse {
	items := make([]transactionResponse, 0, len(transactions))
	for _, transaction := range transactions {
		items = append(items, encodeTransaction(transaction))
	}
	return items
}

func encodeTransaction(transaction core.Transaction) transactionResponse {
	return transactionResponse{
		ID:                 transaction.ID,
		Asset:              transaction.Asset,
		Type:               transaction.TxType.String(),
		From:               transaction.FromAccount,
		To:                 transaction.ToAccount,
		SenderNewStateID:   transaction.SenderNewStateID,
		ReceiverNewStateID: transaction.ReceiverNewStateID,
		Amount:             transaction.Amount.String(),
		Timestamp:          transaction.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func encodeChannel(channel *core.Channel) channelResponse {
	var challengeExpiresAt *string
	if channel.ChallengeExpiresAt != nil {
		value := channel.ChallengeExpiresAt.UTC().Format(time.RFC3339)
		challengeExpiresAt = &value
	}

	return channelResponse{
		ChannelID:             channel.ChannelID,
		UserWallet:            channel.UserWallet,
		Asset:                 channel.Asset,
		Type:                  channelTypeString(channel.Type),
		BlockchainID:          channel.BlockchainID,
		TokenAddress:          channel.TokenAddress,
		ChallengeDuration:     channel.ChallengeDuration,
		ChallengeExpiresAt:    challengeExpiresAt,
		Nonce:                 channel.Nonce,
		ApprovedSigValidators: channel.ApprovedSigValidators,
		Status:                channel.Status.String(),
		StateVersion:          channel.StateVersion,
	}
}

func encodeState(state *core.State) stateResponse {
	var escrowLedger *ledgerResponse
	if state.EscrowLedger != nil {
		encoded := encodeLedger(*state.EscrowLedger)
		escrowLedger = &encoded
	}

	return stateResponse{
		ID:              state.ID,
		Asset:           state.Asset,
		UserWallet:      state.UserWallet,
		Epoch:           state.Epoch,
		Version:         state.Version,
		HomeChannelID:   state.HomeChannelID,
		EscrowChannelID: state.EscrowChannelID,
		Transition: transitionResponse{
			Type:      state.Transition.Type.String(),
			TxID:      state.Transition.TxID,
			AccountID: state.Transition.AccountID,
			Amount:    state.Transition.Amount.String(),
		},
		HomeLedger:   encodeLedger(state.HomeLedger),
		EscrowLedger: escrowLedger,
		UserSig:      state.UserSig,
		NodeSig:      state.NodeSig,
	}
}

func encodeLedger(ledger core.Ledger) ledgerResponse {
	return ledgerResponse{
		TokenAddress: ledger.TokenAddress,
		BlockchainID: ledger.BlockchainID,
		UserBalance:  ledger.UserBalance.String(),
		UserNetFlow:  ledger.UserNetFlow.String(),
		NodeBalance:  ledger.NodeBalance.String(),
		NodeNetFlow:  ledger.NodeNetFlow.String(),
	}
}

func channelTypeString(channelType core.ChannelType) string {
	switch channelType {
	case core.ChannelTypeHome:
		return "home"
	case core.ChannelTypeEscrow:
		return "escrow"
	default:
		return "unknown"
	}
}

func encodeApps(apps []app.AppInfoV1) []appInfoResponse {
	items := make([]appInfoResponse, 0, len(apps))
	for _, item := range apps {
		items = append(items, appInfoResponse{
			AppID:                       item.App.ID,
			OwnerWallet:                 item.App.OwnerWallet,
			Metadata:                    item.App.Metadata,
			Version:                     item.App.Version,
			CreationApprovalNotRequired: item.App.CreationApprovalNotRequired,
			CreatedAt:                   item.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:                   item.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return items
}

func encodeAppParticipants(participants []app.AppParticipantV1) []appParticipantResponse {
	items := make([]appParticipantResponse, 0, len(participants))
	for _, participant := range participants {
		items = append(items, appParticipantResponse{
			WalletAddress:   participant.WalletAddress,
			SignatureWeight: participant.SignatureWeight,
		})
	}
	return items
}

func encodeAppDefinition(definition app.AppDefinitionV1) appDefinitionResponse {
	return appDefinitionResponse{
		ApplicationID: definition.ApplicationID,
		Participants:  encodeAppParticipants(definition.Participants),
		Quorum:        definition.Quorum,
		Nonce:         definition.Nonce,
	}
}

func encodeAppAllocations(allocations []app.AppAllocationV1) []appAllocationResponse {
	items := make([]appAllocationResponse, 0, len(allocations))
	for _, allocation := range allocations {
		items = append(items, appAllocationResponse{
			Participant: allocation.Participant,
			Asset:       allocation.Asset,
			Amount:      allocation.Amount.String(),
		})
	}
	return items
}

func encodeAppSession(session app.AppSessionInfoV1) appSessionResponse {
	status := "open"
	if session.IsClosed {
		status = "closed"
	}
	return appSessionResponse{
		AppSessionID:  session.AppSessionID,
		ApplicationID: session.AppDefinition.ApplicationID,
		Participants:  encodeAppParticipants(session.AppDefinition.Participants),
		Quorum:        session.AppDefinition.Quorum,
		Nonce:         session.AppDefinition.Nonce,
		Status:        status,
		Version:       session.Version,
		SessionData:   session.SessionData,
		Allocations:   encodeAppAllocations(session.Allocations),
	}
}

func encodeAppSessionKeyStates(states []app.AppSessionKeyStateV1) []appSessionKeyStateResponse {
	items := make([]appSessionKeyStateResponse, 0, len(states))
	for _, state := range states {
		items = append(items, appSessionKeyStateResponse{
			UserAddress:    state.UserAddress,
			SessionKey:     state.SessionKey,
			Version:        state.Version,
			ApplicationIDs: append([]string(nil), state.ApplicationIDs...),
			AppSessionIDs:  append([]string(nil), state.AppSessionIDs...),
			ExpiresAt:      state.ExpiresAt.UTC().Format(time.RFC3339),
			UserSig:        state.UserSig,
		})
	}
	return items
}

func encodeChannelSessionKeyStates(states []core.ChannelSessionKeyStateV1) []channelSessionKeyStateResponse {
	items := make([]channelSessionKeyStateResponse, 0, len(states))
	for _, state := range states {
		items = append(items, channelSessionKeyStateResponse{
			UserAddress: state.UserAddress,
			SessionKey:  state.SessionKey,
			Version:     state.Version,
			Assets:      append([]string(nil), state.Assets...),
			ExpiresAt:   state.ExpiresAt.UTC().Format(time.RFC3339),
			UserSig:     state.UserSig,
		})
	}
	return items
}

func encodeDemoGuidance(guidance service.DemoGuidance) demoGuidanceResponse {
	return demoGuidanceResponse{
		NextAction:  guidance.NextAction,
		Headline:    guidance.Headline,
		Description: guidance.Description,
		SyncPending: guidance.SyncPending,
	}
}

func encodeDemoOverview(overview *service.DemoOverview) demoOverviewResponse {
	var channel *channelResponse
	if overview.Channel != nil {
		encoded := encodeChannel(overview.Channel)
		channel = &encoded
	}

	var latestState *stateResponse
	if overview.LatestState != nil {
		encoded := encodeState(overview.LatestState)
		latestState = &encoded
	}

	var latestActivity *transactionResponse
	if overview.LatestActivity != nil {
		encoded := encodeTransaction(*overview.LatestActivity)
		latestActivity = &encoded
	}

	return demoOverviewResponse{
		Status: encodeHealth(overview.Health),
		Wallet: walletResponse{
			Address:         overview.WalletAddress,
			HomeBlockchains: overview.HomeBlockchains,
		},
		SelectedAsset:   overview.SelectedAsset,
		Assets:          encodeAssets(overview.Assets),
		Balances:        encodeBalances(overview.Balances),
		Channel:         channel,
		LatestState:     latestState,
		LatestActivity:  latestActivity,
		Apps:            encodeApps(overview.Apps),
		Sessions:        encodeAppSessions(overview.Sessions),
		ChannelGuidance: encodeDemoGuidance(overview.ChannelGuidance),
		AppGuidance:     encodeDemoGuidance(overview.AppGuidance),
	}
}

func encodeAppSessions(sessions []app.AppSessionInfoV1) []appSessionResponse {
	items := make([]appSessionResponse, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, encodeAppSession(session))
	}
	return items
}
