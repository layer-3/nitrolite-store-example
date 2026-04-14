package httpapi

import (
	"time"

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
		items = append(items, transactionResponse{
			ID:                 transaction.ID,
			Asset:              transaction.Asset,
			Type:               transaction.TxType.String(),
			From:               transaction.FromAccount,
			To:                 transaction.ToAccount,
			SenderNewStateID:   transaction.SenderNewStateID,
			ReceiverNewStateID: transaction.ReceiverNewStateID,
			Amount:             transaction.Amount.String(),
			Timestamp:          transaction.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return items
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
