package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/rpc"
	sdk "github.com/layer-3/nitrolite/sdk/go"
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
	return verifyAppPayloadSignature(walletAddress, payload, signatureHex, func(string) (string, error) {
		return "", conflictf("session keys are not enabled")
	})
}

func (s *WalletStoreService) verifyAppStateSignature(ctx context.Context, walletAddress string, update app.AppStateUpdateV1, signatureHex string) error {
	payload, err := app.PackAppStateUpdateV1(update)
	if err != nil {
		return fmt.Errorf("failed to pack app state update: %w", err)
	}
	return verifyAppPayloadSignature(walletAddress, payload, signatureHex, s.appSessionKeyOwner(ctx, walletAddress, update.AppSessionID))
}

func verifyAppPayloadSignature(walletAddress string, payload []byte, signatureHex string, ownerGetter app.GetAppSessionKeyOwnerFuncV1) error {
	sigBytes, err := hexutil.Decode(strings.TrimSpace(signatureHex))
	if err != nil {
		return invalidCodef("invalid_signature", "invalid user_signature")
	}
	validator := app.NewAppSessionKeySigValidatorV1(ownerGetter)
	if err := validator.Verify(walletAddress, payload, sigBytes); err != nil {
		return conflictCodef("invalid_signature", "invalid app session signature")
	}
	return nil
}

func (s *WalletStoreService) appSessionKeyOwner(ctx context.Context, walletAddress string, appSessionID string) app.GetAppSessionKeyOwnerFuncV1 {
	return func(sessionKeyAddress string) (string, error) {
		client, _, err := activeClient(s.provider)
		if err != nil {
			return "", err
		}
		states, err := client.GetLastAppKeyStates(ctx, walletAddress, &sdk.GetLastKeyStatesOptions{
			SessionKey: &sessionKeyAddress,
		})
		if err != nil {
			return "", err
		}
		if !s.hasActiveAppSessionKeyState(states, walletAddress, sessionKeyAddress, appSessionID) {
			return "", conflictf("session key is not authorized for this store session")
		}
		return walletAddress, nil
	}
}

func (s *WalletStoreService) hasActiveAppSessionKeyState(states []app.AppSessionKeyStateV1, walletAddress string, sessionKeyAddress string, appSessionID string) bool {
	for _, state := range states {
		if !strings.EqualFold(state.UserAddress, walletAddress) {
			continue
		}
		if !strings.EqualFold(state.SessionKey, sessionKeyAddress) {
			continue
		}
		if !state.ExpiresAt.After(s.now()) {
			continue
		}
		for _, allowedSessionID := range state.AppSessionIDs {
			if strings.EqualFold(allowedSessionID, appSessionID) {
				return true
			}
		}
		for _, allowedAppID := range state.ApplicationIDs {
			if strings.EqualFold(allowedAppID, s.appID) {
				return true
			}
		}
	}
	return false
}
