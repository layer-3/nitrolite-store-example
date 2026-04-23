package service

import (
	"context"
	"log/slog"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	appsigning "github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite-go-example/internal/store"
	"github.com/layer-3/nitrolite-go-example/internal/testsupport"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
	"github.com/layer-3/nitrolite/pkg/rpc"
	sdk "github.com/layer-3/nitrolite/sdk/go"
	"github.com/shopspring/decimal"
)

func TestWalletStoreServiceCreateSessionForwardsExactPayload(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	client := &testsupport.FakeClient{
		GetAppsFunc: func(context.Context, *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error) {
			return []app.AppInfoV1{{App: app.AppV1{ID: "store"}}}, core.PaginationMetadata{}, nil
		},
		GetBalancesFunc: func(context.Context, string) ([]core.BalanceEntry, error) {
			return []core.BalanceEntry{{Asset: "yusd", Balance: decimal.RequireFromString("7")}}, nil
		},
	}
	manager := nitrolite.NewManagerWithClient(client, nitrolite.Health{
		Connected:     true,
		Ready:         true,
		SignerAddress: appSigner.Address(),
	}, slog.Default())
	appStore, err := store.New(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	t.Cleanup(func() { _ = appStore.Close() })

	service := NewWalletStoreService(manager, appStore, appSigner, "Nitrolite App Session Store", "store", map[string]uint64{"yusd": 11155111}, "wss://example.invalid")
	service.now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }

	definition := app.AppDefinitionV1{
		ApplicationID: "store",
		Participants: []app.AppParticipantV1{
			{WalletAddress: userSigner.Address(), SignatureWeight: 1},
			{WalletAddress: appSigner.Address(), SignatureWeight: 1},
		},
		Quorum: 2,
		Nonce:  1,
	}
	sessionData := `{"store":"demo"}`
	userSig, err := signCreateAppSessionRequest(definition, sessionData, userSigner)
	if err != nil {
		t.Fatalf("signCreateAppSessionRequest() error = %v", err)
	}

	rpcDefinition := rpc.AppDefinitionV1{
		Application: "store",
		Participants: []rpc.AppParticipantV1{
			{WalletAddress: userSigner.Address(), SignatureWeight: 1},
			{WalletAddress: appSigner.Address(), SignatureWeight: 1},
		},
		Quorum: 2,
		Nonce:  "1",
	}
	currentSession := app.AppSessionInfoV1{
		AppSessionID: "0xsession",
		AppDefinition: definition,
		Version:      1,
		SessionData:  sessionData,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	client.GetAppSessionsFunc = func(context.Context, *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
		return []app.AppSessionInfoV1{currentSession}, core.PaginationMetadata{}, nil
	}

	var captured rpc.AppSessionsV1CreateAppSessionRequest
	service.createAppSessionRPC = func(ctx context.Context, wsURL string, req rpc.AppSessionsV1CreateAppSessionRequest) (*rpc.AppSessionsV1CreateAppSessionResponse, error) {
		captured = req
		return &rpc.AppSessionsV1CreateAppSessionResponse{
			AppSessionID: currentSession.AppSessionID,
			Version:      "1",
			Status:       "open",
		}, nil
	}

	resp, err := service.SubmitUpdate(context.Background(), userSigner.Address(), StoreUpdateRequest{
		Asset:       "yusd",
		Kind:        storeUpdateKindCreateSession,
		Definition:  &rpcDefinition,
		SessionData: sessionData,
		UserSignature: userSig,
	})
	if err != nil {
		t.Fatalf("SubmitUpdate() error = %v", err)
	}

	if !reflect.DeepEqual(captured.Definition, rpcDefinition) {
		t.Fatalf("definition mismatch\n got: %#v\nwant: %#v", captured.Definition, rpcDefinition)
	}
	if captured.SessionData != sessionData {
		t.Fatalf("session_data = %q, want %q", captured.SessionData, sessionData)
	}
	if len(captured.QuorumSigs) != 2 || captured.QuorumSigs[0] != userSig || captured.QuorumSigs[1] == "" {
		t.Fatalf("unexpected quorum signatures = %#v", captured.QuorumSigs)
	}
	if resp.Session.AppSessionID != currentSession.AppSessionID || resp.Session.Status != "open" {
		t.Fatalf("unexpected response session = %#v", resp.Session)
	}
}

func TestWalletStoreServiceSubmitAppStateForwardsExactPayload(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	client := &testsupport.FakeClient{
		GetBalancesFunc: func(context.Context, string) ([]core.BalanceEntry, error) {
			return []core.BalanceEntry{{Asset: "yusd", Balance: decimal.RequireFromString("7")}}, nil
		},
	}
	manager := nitrolite.NewManagerWithClient(client, nitrolite.Health{
		Connected:     true,
		Ready:         true,
		SignerAddress: appSigner.Address(),
	}, slog.Default())
	appStore, err := store.New(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	t.Cleanup(func() { _ = appStore.Close() })

	service := NewWalletStoreService(manager, appStore, appSigner, "Nitrolite App Session Store", "store", map[string]uint64{"yusd": 11155111}, "wss://example.invalid")
	service.now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }

	currentSession := app.AppSessionInfoV1{
		AppSessionID: "0xsession",
		AppDefinition: app.AppDefinitionV1{
			ApplicationID: "store",
			Participants: []app.AppParticipantV1{
				{WalletAddress: userSigner.Address(), SignatureWeight: 1},
				{WalletAddress: appSigner.Address(), SignatureWeight: 1},
			},
			Quorum: 2,
			Nonce:  1,
		},
		Version:     1,
		SessionData: `{"action":"deposit","amount":"1.00"}`,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1.0")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	client.GetAppSessionsFunc = func(context.Context, *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
		return []app.AppSessionInfoV1{currentSession}, core.PaginationMetadata{}, nil
	}
	if err := appStore.UpsertWalletSession(context.Background(), store.WalletStoreSession{
		WalletAddress:  userSigner.Address(),
		Asset:          "yusd",
		AppSessionID:   currentSession.AppSessionID,
		Status:         "open",
		Version:        currentSession.Version,
		UserAllocation: "1",
		AppAllocation:  "0",
		SessionData:    currentSession.SessionData,
		CreatedAt:      time.Unix(1_700_000_000, 0).UTC(),
		UpdatedAt:      time.Unix(1_700_000_000, 0).UTC(),
	}); err != nil {
		t.Fatalf("UpsertWalletSession() error = %v", err)
	}

	update := app.AppStateUpdateV1{
		AppSessionID: currentSession.AppSessionID,
		Intent:       app.AppStateUpdateIntentWithdraw,
		Version:      2,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.5")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
		SessionData: `{"action":"user_withdraw","amount":"0.50"}`,
	}
	userSig, err := signAppStateUpdate(update, userSigner)
	if err != nil {
		t.Fatalf("signAppStateUpdate() error = %v", err)
	}
	rpcUpdate := rpc.AppStateUpdateV1{
		AppSessionID: update.AppSessionID,
		Intent:       update.Intent,
		Version:      "2",
		Allocations: []rpc.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: "0.5"},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: "0"},
		},
		SessionData: update.SessionData,
	}

	var captured rpc.AppSessionsV1SubmitAppStateRequest
	service.submitAppStateRPC = func(ctx context.Context, wsURL string, req rpc.AppSessionsV1SubmitAppStateRequest) error {
		captured = req
		currentSession.Version = update.Version
		currentSession.SessionData = update.SessionData
		currentSession.Allocations = update.Allocations
		return nil
	}

	resp, err := service.SubmitUpdate(context.Background(), userSigner.Address(), StoreUpdateRequest{
		Asset:          "yusd",
		Kind:           storeUpdateKindAppState,
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	})
	if err != nil {
		t.Fatalf("SubmitUpdate() error = %v", err)
	}

	if !reflect.DeepEqual(captured.AppStateUpdate, rpcUpdate) {
		t.Fatalf("app_state_update mismatch\n got: %#v\nwant: %#v", captured.AppStateUpdate, rpcUpdate)
	}
	if len(captured.QuorumSigs) != 2 || captured.QuorumSigs[0] != userSig || captured.QuorumSigs[1] == "" {
		t.Fatalf("unexpected quorum signatures = %#v", captured.QuorumSigs)
	}
	if resp.Session.Version != 2 || resp.Session.UserAllocation != "0.5" {
		t.Fatalf("unexpected response session = %#v", resp.Session)
	}
}

func TestWalletStoreServiceSubmitPurchaseAcceptsNormalizedPriceString(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	client := &testsupport.FakeClient{
		GetBalancesFunc: func(context.Context, string) ([]core.BalanceEntry, error) {
			return []core.BalanceEntry{{Asset: "yusd", Balance: decimal.RequireFromString("7")}}, nil
		},
	}
	manager := nitrolite.NewManagerWithClient(client, nitrolite.Health{
		Connected:     true,
		Ready:         true,
		SignerAddress: appSigner.Address(),
	}, slog.Default())
	appStore, err := store.New(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	t.Cleanup(func() { _ = appStore.Close() })

	service := NewWalletStoreService(manager, appStore, appSigner, "Nitrolite App Session Store", "store", map[string]uint64{"yusd": 11155111}, "wss://example.invalid")
	service.now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }

	currentSession := app.AppSessionInfoV1{
		AppSessionID: "0xsession",
		AppDefinition: app.AppDefinitionV1{
			ApplicationID: "store",
			Participants: []app.AppParticipantV1{
				{WalletAddress: userSigner.Address(), SignatureWeight: 1},
				{WalletAddress: appSigner.Address(), SignatureWeight: 1},
			},
			Quorum: 2,
			Nonce:  1,
		},
		Version:     1,
		SessionData: `{"action":"deposit","amount":"1.00"}`,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1.0")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	client.GetAppSessionsFunc = func(context.Context, *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
		return []app.AppSessionInfoV1{currentSession}, core.PaginationMetadata{}, nil
	}
	if err := appStore.UpsertWalletSession(context.Background(), store.WalletStoreSession{
		WalletAddress:  userSigner.Address(),
		Asset:          "yusd",
		AppSessionID:   currentSession.AppSessionID,
		Status:         "open",
		Version:        currentSession.Version,
		UserAllocation: "1",
		AppAllocation:  "0",
		SessionData:    currentSession.SessionData,
		CreatedAt:      time.Unix(1_700_000_000, 0).UTC(),
		UpdatedAt:      time.Unix(1_700_000_000, 0).UTC(),
	}); err != nil {
		t.Fatalf("UpsertWalletSession() error = %v", err)
	}

	update := app.AppStateUpdateV1{
		AppSessionID: currentSession.AppSessionID,
		Intent:       app.AppStateUpdateIntentOperate,
		Version:      2,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.5")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.5")},
		},
		SessionData: `{"action":"purchase","item_id":"article-micropayments","price":"0.5"}`,
	}
	userSig, err := signAppStateUpdate(update, userSigner)
	if err != nil {
		t.Fatalf("signAppStateUpdate() error = %v", err)
	}
	rpcUpdate := rpc.AppStateUpdateV1{
		AppSessionID: update.AppSessionID,
		Intent:       update.Intent,
		Version:      "2",
		Allocations: []rpc.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: "0.5"},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: "0.5"},
		},
		SessionData: update.SessionData,
	}

	service.submitAppStateRPC = func(context.Context, string, rpc.AppSessionsV1SubmitAppStateRequest) error {
		currentSession.Version = update.Version
		currentSession.SessionData = update.SessionData
		currentSession.Allocations = update.Allocations
		return nil
	}

	resp, err := service.SubmitUpdate(context.Background(), userSigner.Address(), StoreUpdateRequest{
		Asset:          "yusd",
		Kind:           storeUpdateKindAppState,
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	})
	if err != nil {
		t.Fatalf("SubmitUpdate() error = %v", err)
	}

	if len(resp.Library) != 1 || resp.Library[0].ID != "article-micropayments" {
		t.Fatalf("unexpected library = %#v", resp.Library)
	}
}

func mustStoreAppSigner(t *testing.T) appsigning.Signer {
	t.Helper()

	signer, err := appsigning.NewStoreAppSigner("", serviceTestPrivateKey)
	if err != nil {
		t.Fatalf("NewStoreAppSigner() error = %v", err)
	}
	return signer
}
