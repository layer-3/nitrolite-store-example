package service

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	appsigning "github.com/layer-3/nitrolite-store-example/internal/signing"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/sign"
	"github.com/shopspring/decimal"
)

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

func parsePositiveAmount(raw string) (decimal.Decimal, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil {
		return decimal.Decimal{}, invalidf("invalid amount")
	}
	if !amount.IsPositive() {
		return decimal.Decimal{}, invalidf("amount must be positive")
	}
	return amount, nil
}
