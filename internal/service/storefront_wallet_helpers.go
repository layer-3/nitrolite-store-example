package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
	"github.com/layer-3/nitrolite/pkg/rpc"
	"github.com/shopspring/decimal"
)

func appDefinitionFromRPC(def rpc.AppDefinitionV1) (app.AppDefinitionV1, error) {
	participants := make([]app.AppParticipantV1, 0, len(def.Participants))
	for _, participant := range def.Participants {
		participants = append(participants, app.AppParticipantV1{
			WalletAddress:   participant.WalletAddress,
			SignatureWeight: participant.SignatureWeight,
		})
	}

	nonce, err := strconv.ParseUint(def.Nonce, 10, 64)
	if err != nil {
		return app.AppDefinitionV1{}, invalidf("invalid create session nonce")
	}

	return app.AppDefinitionV1{
		ApplicationID: def.Application,
		Participants:  participants,
		Quorum:        def.Quorum,
		Nonce:         nonce,
	}, nil
}

func appStateUpdateFromRPC(update rpc.AppStateUpdateV1) (app.AppStateUpdateV1, error) {
	version, err := strconv.ParseUint(update.Version, 10, 64)
	if err != nil {
		return app.AppStateUpdateV1{}, invalidf("invalid app_state_update.version")
	}

	allocations := make([]app.AppAllocationV1, 0, len(update.Allocations))
	for _, allocation := range update.Allocations {
		amount, err := decimal.NewFromString(strings.TrimSpace(allocation.Amount))
		if err != nil {
			return app.AppStateUpdateV1{}, invalidf("invalid app_state_update allocation amount")
		}
		allocations = append(allocations, app.AppAllocationV1{
			Participant: strings.TrimSpace(allocation.Participant),
			Asset:       strings.ToLower(strings.TrimSpace(allocation.Asset)),
			Amount:      amount,
		})
	}

	return app.AppStateUpdateV1{
		AppSessionID: strings.TrimSpace(update.AppSessionID),
		Intent:       update.Intent,
		Version:      version,
		Allocations:  allocations,
		SessionData:  update.SessionData,
	}, nil
}

func stateFromRPC(state rpc.StateV1) (core.State, error) {
	epoch, err := strconv.ParseUint(state.Epoch, 10, 64)
	if err != nil {
		return core.State{}, invalidf("invalid user_state.epoch")
	}
	version, err := strconv.ParseUint(state.Version, 10, 64)
	if err != nil {
		return core.State{}, invalidf("invalid user_state.version")
	}

	homeLedger, err := ledgerFromRPC(state.HomeLedger)
	if err != nil {
		return core.State{}, err
	}

	var escrowLedger *core.Ledger
	if state.EscrowLedger != nil {
		ledger, err := ledgerFromRPC(*state.EscrowLedger)
		if err != nil {
			return core.State{}, err
		}
		escrowLedger = &ledger
	}

	amount, err := decimal.NewFromString(strings.TrimSpace(state.Transition.Amount))
	if err != nil {
		return core.State{}, invalidf("invalid user_state.transition.amount")
	}

	return core.State{
		ID:              state.ID,
		Transition:      core.Transition{Type: state.Transition.Type, TxID: state.Transition.TxID, AccountID: state.Transition.AccountID, Amount: amount},
		Asset:           strings.ToLower(strings.TrimSpace(state.Asset)),
		UserWallet:      strings.TrimSpace(state.UserWallet),
		Epoch:           epoch,
		Version:         version,
		HomeChannelID:   state.HomeChannelID,
		EscrowChannelID: state.EscrowChannelID,
		HomeLedger:      homeLedger,
		EscrowLedger:    escrowLedger,
		UserSig:         state.UserSig,
		NodeSig:         state.NodeSig,
	}, nil
}

func ledgerFromRPC(ledger rpc.LedgerV1) (core.Ledger, error) {
	blockchainID, err := strconv.ParseUint(strings.TrimSpace(ledger.BlockchainID), 10, 64)
	if err != nil {
		return core.Ledger{}, invalidf("invalid user_state ledger blockchain_id")
	}
	userBalance, err := decimal.NewFromString(strings.TrimSpace(ledger.UserBalance))
	if err != nil {
		return core.Ledger{}, invalidf("invalid user_state ledger user_balance")
	}
	userNetFlow, err := decimal.NewFromString(strings.TrimSpace(ledger.UserNetFlow))
	if err != nil {
		return core.Ledger{}, invalidf("invalid user_state ledger user_net_flow")
	}
	nodeBalance, err := decimal.NewFromString(strings.TrimSpace(ledger.NodeBalance))
	if err != nil {
		return core.Ledger{}, invalidf("invalid user_state ledger node_balance")
	}
	nodeNetFlow, err := decimal.NewFromString(strings.TrimSpace(ledger.NodeNetFlow))
	if err != nil {
		return core.Ledger{}, invalidf("invalid user_state ledger node_net_flow")
	}

	return core.Ledger{
		TokenAddress: ledger.TokenAddress,
		BlockchainID: blockchainID,
		UserBalance:  userBalance,
		UserNetFlow:  userNetFlow,
		NodeBalance:  nodeBalance,
		NodeNetFlow:  nodeNetFlow,
	}, nil
}

func verifyCreateSessionSignature(walletAddress string, definition app.AppDefinitionV1, sessionData string, signatureHex string) error {
	payload, err := app.PackCreateAppSessionRequestV1(definition, sessionData)
	if err != nil {
		return fmt.Errorf("failed to pack create app session request: %w", err)
	}
	return verifyWalletAppPayloadSignature(walletAddress, payload, signatureHex)
}

func verifyAppStateSignature(walletAddress string, update app.AppStateUpdateV1, signatureHex string) error {
	payload, err := app.PackAppStateUpdateV1(update)
	if err != nil {
		return fmt.Errorf("failed to pack app state update: %w", err)
	}
	return verifyWalletAppPayloadSignature(walletAddress, payload, signatureHex)
}

func verifyWalletAppPayloadSignature(walletAddress string, payload []byte, signatureHex string) error {
	sigBytes, err := hexutil.Decode(strings.TrimSpace(signatureHex))
	if err != nil {
		return invalidf("invalid user_signature")
	}
	validator := app.NewAppSessionKeySigValidatorV1(func(string) (string, error) {
		return "", conflictf("session keys are not enabled")
	})
	if err := validator.Verify(walletAddress, payload, sigBytes); err != nil {
		return conflictf("invalid app session signature")
	}
	return nil
}

func verifyChannelStateSignature(walletAddress string, state core.State, assetStore core.AssetStore) error {
	if state.UserSig == nil {
		return conflictf("user_state is missing user_sig")
	}
	sigBytes, err := hexutil.Decode(strings.TrimSpace(*state.UserSig))
	if err != nil {
		return invalidf("invalid user_state.user_sig")
	}
	packedState, err := core.PackState(state, assetStore)
	if err != nil {
		return fmt.Errorf("failed to pack user_state: %w", err)
	}
	validator := core.NewChannelSigValidator(func(string, string, string) (bool, error) {
		return false, nil
	})
	if err := validator.Verify(walletAddress, packedState, sigBytes); err != nil {
		return conflictf("invalid user_state signature")
	}
	return nil
}

type staticAssetStore struct {
	assetDecimals map[string]uint8
	tokenDecimals map[string]uint8
}

func (s staticAssetStore) GetAssetDecimals(asset string) (uint8, error) {
	if decimals, ok := s.assetDecimals[strings.ToLower(strings.TrimSpace(asset))]; ok {
		return decimals, nil
	}
	return 0, fmt.Errorf("asset decimals not found")
}

func (s staticAssetStore) GetTokenDecimals(blockchainID uint64, tokenAddress string) (uint8, error) {
	key := fmt.Sprintf("%d::%s", blockchainID, strings.ToLower(strings.TrimSpace(tokenAddress)))
	if decimals, ok := s.tokenDecimals[key]; ok {
		return decimals, nil
	}
	return 0, fmt.Errorf("token decimals not found")
}

func (s *WalletStoreService) assetStore(ctx context.Context) (core.AssetStore, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}
	assets, err := client.GetAssets(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to load asset metadata: %w", err)
	}

	store := staticAssetStore{
		assetDecimals: make(map[string]uint8, len(assets)),
		tokenDecimals: make(map[string]uint8),
	}
	for _, asset := range assets {
		store.assetDecimals[strings.ToLower(asset.Symbol)] = asset.Decimals
		for _, token := range asset.Tokens {
			key := fmt.Sprintf("%d::%s", token.BlockchainID, strings.ToLower(token.Address))
			store.tokenDecimals[key] = token.Decimals
		}
	}
	return store, nil
}
