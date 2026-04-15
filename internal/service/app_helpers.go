package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	appsigning "github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/sign"
	"github.com/shopspring/decimal"
)

func validateAppID(appID string) error {
	if !app.AppIDV1Regex.MatchString(strings.TrimSpace(appID)) {
		return invalidf("invalid app_id")
	}
	return nil
}

func buildSingleParticipantAppDefinition(appID string, signerAddress string, nonce uint64) app.AppDefinitionV1 {
	return app.AppDefinitionV1{
		ApplicationID: appID,
		Participants: []app.AppParticipantV1{
			{
				WalletAddress:   signerAddress,
				SignatureWeight: 1,
			},
		},
		Quorum: 1,
		Nonce:  nonce,
	}
}

func buildInitialAllocations(signerAddress string, allocations []InitialAllocationInput) ([]app.AppAllocationV1, error) {
	if len(allocations) == 0 {
		return nil, invalidf("initial_allocations must not be empty")
	}

	out := make([]app.AppAllocationV1, 0, len(allocations))
	for _, allocation := range allocations {
		asset := strings.TrimSpace(strings.ToLower(allocation.Asset))
		if asset == "" {
			return nil, invalidf("asset is required")
		}
		if !allocation.Amount.IsPositive() {
			return nil, invalidf("amount must be positive")
		}

		out = append(out, app.AppAllocationV1{
			Participant: signerAddress,
			Asset:       asset,
			Amount:      allocation.Amount,
		})
	}

	sortAppAllocations(out)
	return out, nil
}

func buildOperateAllocations(defaultParticipant string, allocations []AllocationInput) ([]app.AppAllocationV1, error) {
	if len(allocations) == 0 {
		return nil, invalidf("allocations must not be empty")
	}

	out := make([]app.AppAllocationV1, 0, len(allocations))
	for _, allocation := range allocations {
		participant := strings.TrimSpace(allocation.Participant)
		if participant == "" {
			participant = defaultParticipant
		}
		asset := strings.TrimSpace(strings.ToLower(allocation.Asset))
		if participant == "" {
			return nil, invalidf("participant is required")
		}
		if asset == "" {
			return nil, invalidf("asset is required")
		}
		if !allocation.Amount.IsPositive() {
			return nil, invalidf("amount must be positive")
		}

		out = append(out, app.AppAllocationV1{
			Participant: participant,
			Asset:       asset,
			Amount:      allocation.Amount,
		})
	}

	sortAppAllocations(out)
	return out, nil
}

func buildDepositUpdate(current app.AppSessionInfoV1, signerAddress string, asset string, amount decimal.Decimal) (app.AppStateUpdateV1, error) {
	normalizedAsset := strings.TrimSpace(strings.ToLower(asset))
	if normalizedAsset == "" {
		return app.AppStateUpdateV1{}, invalidf("asset is required")
	}
	if !amount.IsPositive() {
		return app.AppStateUpdateV1{}, invalidf("amount must be positive")
	}

	allocations := cloneAllocations(current.Allocations)
	found := false
	for i := range allocations {
		if strings.EqualFold(allocations[i].Participant, signerAddress) && strings.EqualFold(allocations[i].Asset, normalizedAsset) {
			allocations[i].Amount = allocations[i].Amount.Add(amount)
			found = true
			break
		}
	}
	if !found {
		allocations = append(allocations, app.AppAllocationV1{
			Participant: signerAddress,
			Asset:       normalizedAsset,
			Amount:      amount,
		})
	}

	sortAppAllocations(allocations)
	return app.AppStateUpdateV1{
		AppSessionID: current.AppSessionID,
		Intent:       app.AppStateUpdateIntentDeposit,
		Version:      current.Version + 1,
		Allocations:  allocations,
		SessionData:  current.SessionData,
	}, nil
}

func buildOperateUpdate(current app.AppSessionInfoV1, allocations []app.AppAllocationV1, sessionData string) (app.AppStateUpdateV1, error) {
	if len(allocations) == 0 {
		return app.AppStateUpdateV1{}, invalidf("allocations must not be empty")
	}
	allocations = cloneAllocations(allocations)
	sortAppAllocations(allocations)
	return app.AppStateUpdateV1{
		AppSessionID: current.AppSessionID,
		Intent:       app.AppStateUpdateIntentOperate,
		Version:      current.Version + 1,
		Allocations:  allocations,
		SessionData:  sessionData,
	}, nil
}

func buildCloseUpdate(current app.AppSessionInfoV1) (app.AppStateUpdateV1, error) {
	if len(current.Allocations) == 0 {
		return app.AppStateUpdateV1{}, invalidf("session has no allocations")
	}

	allocations := cloneAllocations(current.Allocations)
	sortAppAllocations(allocations)
	return app.AppStateUpdateV1{
		AppSessionID: current.AppSessionID,
		Intent:       app.AppStateUpdateIntentClose,
		Version:      current.Version + 1,
		Allocations:  allocations,
		SessionData:  current.SessionData,
	}, nil
}

func signCreateAppSessionRequest(definition app.AppDefinitionV1, sessionData string, signer appsigning.Signer) (string, error) {
	payload, err := app.PackCreateAppSessionRequestV1(definition, sessionData)
	if err != nil {
		return "", fmt.Errorf("failed to pack create app session request: %w", err)
	}

	signerImpl, err := newAppSessionWalletSigner(signer)
	if err != nil {
		return "", err
	}

	signature, err := signerImpl.Sign(payload)
	if err != nil {
		return "", fmt.Errorf("failed to sign create app session request: %w", err)
	}
	return signature.String(), nil
}

func signAppStateUpdate(update app.AppStateUpdateV1, signer appsigning.Signer) (string, error) {
	payload, err := app.PackAppStateUpdateV1(update)
	if err != nil {
		return "", fmt.Errorf("failed to pack app state update: %w", err)
	}

	signerImpl, err := newAppSessionWalletSigner(signer)
	if err != nil {
		return "", err
	}

	signature, err := signerImpl.Sign(payload)
	if err != nil {
		return "", fmt.Errorf("failed to sign app state update: %w", err)
	}
	return signature.String(), nil
}

func newAppSessionWalletSigner(signer appsigning.Signer) (*app.AppSessionSignerV1, error) {
	msgSigner, err := sign.NewEthereumMsgSignerFromRaw(signer.TxSigner())
	if err != nil {
		return nil, fmt.Errorf("failed to create app session message signer: %w", err)
	}
	appSessionSigner, err := app.NewAppSessionWalletSignerV1(msgSigner)
	if err != nil {
		return nil, fmt.Errorf("failed to create app session wallet signer: %w", err)
	}
	return appSessionSigner, nil
}

func parseNumericVersion(raw string) (uint64, error) {
	value, err := hexutil.DecodeUint64(raw)
	if err == nil {
		return value, nil
	}

	parsed, parseErr := decimal.NewFromString(raw)
	if parseErr != nil {
		return 0, invalidf("invalid version")
	}
	if !parsed.IsInteger() || parsed.IsNegative() {
		return 0, invalidf("invalid version")
	}
	return uint64(parsed.IntPart()), nil
}

func cloneAllocations(in []app.AppAllocationV1) []app.AppAllocationV1 {
	out := make([]app.AppAllocationV1, 0, len(in))
	for _, allocation := range in {
		out = append(out, app.AppAllocationV1{
			Participant: allocation.Participant,
			Asset:       allocation.Asset,
			Amount:      allocation.Amount,
		})
	}
	return out
}

func sortAppAllocations(allocations []app.AppAllocationV1) {
	sort.Slice(allocations, func(i, j int) bool {
		if allocations[i].Participant == allocations[j].Participant {
			return allocations[i].Asset < allocations[j].Asset
		}
		return allocations[i].Participant < allocations[j].Participant
	})
}
