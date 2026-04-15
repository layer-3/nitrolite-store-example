package service

import (
	"context"
	"log/slog"
	"testing"

	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	appsigning "github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite-go-example/internal/testsupport"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
	sdk "github.com/layer-3/nitrolite/sdk/go"
	"github.com/shopspring/decimal"
)

func TestAppSessionService_RegisterApp(t *testing.T) {
	t.Parallel()

	var called bool
	svc := newTestAppSessionService(t, &testsupport.FakeClient{
		RegisterAppFunc: func(ctx context.Context, appID string, metadata string, creationApprovalNotRequired bool) error {
			called = true
			if appID != "demo-app" || metadata != `{"name":"Demo"}` || !creationApprovalNotRequired {
				t.Fatalf("RegisterApp args = %q %q %v", appID, metadata, creationApprovalNotRequired)
			}
			return nil
		},
	})

	if err := svc.RegisterApp(context.Background(), "demo-app", `{"name":"Demo"}`, true); err != nil {
		t.Fatalf("RegisterApp() error = %v", err)
	}
	if !called {
		t.Fatal("RegisterApp was not called")
	}
}

func TestAppSessionService_CreateSession(t *testing.T) {
	t.Parallel()

	var gotDefinition app.AppDefinitionV1
	var gotSigCount int
	var deposited app.AppStateUpdateV1
	svc := newTestAppSessionService(t, &testsupport.FakeClient{
		GetAppsFunc: func(ctx context.Context, opts *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error) {
			return []app.AppInfoV1{
				{App: app.AppV1{ID: "demo-app", CreationApprovalNotRequired: true}},
			}, core.PaginationMetadata{}, nil
		},
		CreateAppSessionFunc: func(ctx context.Context, definition app.AppDefinitionV1, sessionData string, quorumSigs []string, opts ...sdk.CreateAppSessionOptions) (string, string, string, error) {
			gotDefinition = definition
			gotSigCount = len(quorumSigs)
			return "0xsession", "1", "open", nil
		},
		SubmitAppSessionDepositFunc: func(ctx context.Context, update app.AppStateUpdateV1, quorumSigs []string, asset string, amount decimal.Decimal) (string, error) {
			deposited = update
			return "0xnodesig", nil
		},
	})

	created, err := svc.CreateSession(context.Background(), CreateAppSessionRequest{
		ApplicationID: "demo-app",
		InitialAllocations: []InitialAllocationInput{
			{Asset: "usdc", Amount: decimal.RequireFromString("5")},
		},
		SessionData: "{}",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if created.SessionID != "0xsession" || created.Version != 2 || created.Status != "open" {
		t.Fatalf("created = %#v", created)
	}
	if gotDefinition.Nonce != 42 || gotSigCount != 1 {
		t.Fatalf("definition/signatures = %#v %d", gotDefinition, gotSigCount)
	}
	if deposited.Intent != app.AppStateUpdateIntentDeposit || len(deposited.Allocations) != 1 {
		t.Fatalf("deposited = %#v", deposited)
	}
	if deposited.Allocations[0].Amount.String() != "5" {
		t.Fatalf("deposit allocations = %#v", deposited.Allocations)
	}
}

func TestAppSessionService_DepositOperateClose(t *testing.T) {
	t.Parallel()

	current := app.AppSessionInfoV1{
		AppSessionID: "0xsession",
		AppDefinition: app.AppDefinitionV1{
			ApplicationID: "demo-app",
			Participants:  []app.AppParticipantV1{{WalletAddress: serviceTestAddress, SignatureWeight: 1}},
			Quorum:        1,
			Nonce:         42,
		},
		IsClosed:    false,
		SessionData: "{}",
		Version:     1,
		Allocations: []app.AppAllocationV1{{Participant: serviceTestAddress, Asset: "usdc", Amount: decimal.RequireFromString("5")}},
	}

	var deposited app.AppStateUpdateV1
	var operated app.AppStateUpdateV1
	var closed app.AppStateUpdateV1
	svc := newTestAppSessionService(t, &testsupport.FakeClient{
		GetAppSessionsFunc: func(ctx context.Context, opts *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
			return []app.AppSessionInfoV1{current}, core.PaginationMetadata{}, nil
		},
		SubmitAppSessionDepositFunc: func(ctx context.Context, update app.AppStateUpdateV1, quorumSigs []string, asset string, amount decimal.Decimal) (string, error) {
			deposited = update
			return "0xnodesig", nil
		},
		SubmitAppStateFunc: func(ctx context.Context, update app.AppStateUpdateV1, quorumSigs []string) error {
			switch update.Intent {
			case app.AppStateUpdateIntentOperate:
				operated = update
			case app.AppStateUpdateIntentClose:
				closed = update
			}
			return nil
		},
	})

	depositResult, err := svc.DepositSession(context.Background(), "0xsession", DepositAppSessionRequest{
		Asset:  "usdc",
		Amount: decimal.RequireFromString("2"),
	})
	if err != nil {
		t.Fatalf("DepositSession() error = %v", err)
	}
	if depositResult.Version != 2 || depositResult.NodeSig != "0xnodesig" {
		t.Fatalf("depositResult = %#v", depositResult)
	}
	if deposited.Allocations[0].Amount.String() != "7" {
		t.Fatalf("deposited allocations = %#v", deposited.Allocations)
	}

	operateResult, err := svc.OperateSession(context.Background(), "0xsession", OperateAppSessionRequest{
		Allocations: []AllocationInput{
			{Asset: "usdc", Amount: decimal.RequireFromString("8")},
		},
		SessionData: `{"turn":2}`,
	})
	if err != nil {
		t.Fatalf("OperateSession() error = %v", err)
	}
	if operateResult.Version != 2 || operated.Intent != app.AppStateUpdateIntentOperate {
		t.Fatalf("operate = %#v %#v", operateResult, operated)
	}

	closeResult, err := svc.CloseSession(context.Background(), "0xsession")
	if err != nil {
		t.Fatalf("CloseSession() error = %v", err)
	}
	if closeResult.Status != "closed" || closed.Intent != app.AppStateUpdateIntentClose {
		t.Fatalf("close = %#v %#v", closeResult, closed)
	}
}

func TestAppSessionService_GetSessionsDefaultsToSignerParticipant(t *testing.T) {
	t.Parallel()

	var gotParticipant *string
	svc := newTestAppSessionService(t, &testsupport.FakeClient{
		GetAppSessionsFunc: func(ctx context.Context, opts *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
			gotParticipant = opts.Participant
			return []app.AppSessionInfoV1{}, core.PaginationMetadata{}, nil
		},
	})

	_, _, err := svc.GetSessions(context.Background(), SessionListFilter{
		Status:  "all",
		Page:    1,
		PerPage: 20,
	})
	if err != nil {
		t.Fatalf("GetSessions() error = %v", err)
	}
	if gotParticipant == nil || *gotParticipant != serviceTestAddress {
		t.Fatalf("participant = %v, want %s", gotParticipant, serviceTestAddress)
	}
}

func newTestAppSessionService(t *testing.T, client *testsupport.FakeClient) *AppSessionService {
	t.Helper()

	if client.GetUserAddressFunc == nil {
		client.GetUserAddressFunc = func() string { return serviceTestAddress }
	}
	manager := nitrolite.NewManagerWithClient(client, nitrolite.Health{
		Connected:     true,
		Ready:         true,
		SignerAddress: serviceTestAddress,
	}, slog.Default())
	signer := mustTestServiceSigner(t)
	return NewAppSessionService(manager, signer, func() uint64 { return 42 })
}

func mustTestServiceSigner(t *testing.T) appsigning.Signer {
	t.Helper()
	signer, err := appsigning.NewEnvSigner(serviceTestPrivateKey)
	if err != nil {
		t.Fatalf("NewEnvSigner() error = %v", err)
	}
	return signer
}
