package service

import (
	"encoding/hex"
	"testing"

	appsigning "github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/sign"
	"github.com/shopspring/decimal"
)

const (
	serviceTestPrivateKey = "0x4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"
	serviceTestAddress    = "0x2c7536e3605d9c16a7a3d7b1898e529396a65c23"
)

func TestValidateAppID(t *testing.T) {
	t.Parallel()

	if err := validateAppID("demo-app"); err != nil {
		t.Fatalf("validateAppID(valid) error = %v", err)
	}
	if err := validateAppID("Demo App"); err == nil {
		t.Fatal("validateAppID(invalid) error = nil")
	}
}

func TestBuildSingleParticipantAppDefinition(t *testing.T) {
	t.Parallel()

	definition := buildSingleParticipantAppDefinition("demo-app", serviceTestAddress, 42)
	if definition.ApplicationID != "demo-app" {
		t.Fatalf("ApplicationID = %s", definition.ApplicationID)
	}
	if len(definition.Participants) != 1 || definition.Participants[0].WalletAddress != serviceTestAddress {
		t.Fatalf("Participants = %#v", definition.Participants)
	}
	if definition.Quorum != 1 || definition.Nonce != 42 {
		t.Fatalf("definition = %#v", definition)
	}
}

func TestSignCreateAppSessionRequest(t *testing.T) {
	t.Parallel()

	signer := mustTestSigner(t)
	definition := buildSingleParticipantAppDefinition("demo-app", signer.Address(), 7)

	signatureHex, err := signCreateAppSessionRequest(definition, "{}", signer)
	if err != nil {
		t.Fatalf("signCreateAppSessionRequest() error = %v", err)
	}

	payload, err := app.PackCreateAppSessionRequestV1(definition, "{}")
	if err != nil {
		t.Fatalf("PackCreateAppSessionRequestV1() error = %v", err)
	}
	verifyAppSessionWalletSignature(t, payload, signatureHex, signer.Address())
}

func TestBuildDepositUpdate(t *testing.T) {
	t.Parallel()

	current := app.AppSessionInfoV1{
		AppSessionID: "0xsession",
		Version:      3,
		SessionData:  "{}",
		Allocations: []app.AppAllocationV1{
			{Participant: serviceTestAddress, Asset: "usdc", Amount: decimal.RequireFromString("5")},
		},
	}

	update, err := buildDepositUpdate(current, serviceTestAddress, "usdc", decimal.RequireFromString("2"))
	if err != nil {
		t.Fatalf("buildDepositUpdate() error = %v", err)
	}
	if update.Version != 4 {
		t.Fatalf("Version = %d, want 4", update.Version)
	}
	if got := update.Allocations[0].Amount.String(); got != "7" {
		t.Fatalf("Amount = %s, want 7", got)
	}
}

func TestBuildOperateAndCloseUpdates(t *testing.T) {
	t.Parallel()

	current := app.AppSessionInfoV1{
		AppSessionID: "0xsession",
		Version:      1,
		SessionData:  "{}",
		Allocations: []app.AppAllocationV1{
			{Participant: serviceTestAddress, Asset: "usdc", Amount: decimal.RequireFromString("5")},
		},
	}

	operate, err := buildOperateUpdate(current, []app.AppAllocationV1{
		{Participant: serviceTestAddress, Asset: "usdc", Amount: decimal.RequireFromString("8")},
	}, `{"turn":2}`)
	if err != nil {
		t.Fatalf("buildOperateUpdate() error = %v", err)
	}
	if operate.Intent != app.AppStateUpdateIntentOperate || operate.Version != 2 {
		t.Fatalf("operate = %#v", operate)
	}

	closeUpdate, err := buildCloseUpdate(current)
	if err != nil {
		t.Fatalf("buildCloseUpdate() error = %v", err)
	}
	if closeUpdate.Intent != app.AppStateUpdateIntentClose || closeUpdate.Version != 2 {
		t.Fatalf("closeUpdate = %#v", closeUpdate)
	}
}

func mustTestSigner(t *testing.T) appsigning.Signer {
	t.Helper()

	signer, err := appsigning.NewEnvSigner(serviceTestPrivateKey)
	if err != nil {
		t.Fatalf("NewEnvSigner() error = %v", err)
	}
	return signer
}

func verifyAppSessionWalletSignature(t *testing.T, payload []byte, signatureHex string, wallet string) {
	t.Helper()

	signatureBytes, err := hex.DecodeString(signatureHex[2:])
	if err != nil {
		t.Fatalf("hex.DecodeString() error = %v", err)
	}

	validator := app.NewAppSessionKeySigValidatorV1(func(sessionKeyAddr string) (string, error) {
		return sessionKeyAddr, nil
	})
	if err := validator.Verify(wallet, payload, sign.Signature(signatureBytes)); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}
