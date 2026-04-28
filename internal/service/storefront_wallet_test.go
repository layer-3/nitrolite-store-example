package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/layer-3/nitrolite-store-example/internal/nitrolite"
	appsigning "github.com/layer-3/nitrolite-store-example/internal/signing"
	"github.com/layer-3/nitrolite-store-example/internal/store"
	"github.com/layer-3/nitrolite-store-example/internal/testsupport"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
	"github.com/layer-3/nitrolite/pkg/rpc"
	sdk "github.com/layer-3/nitrolite/sdk/go"
	"github.com/shopspring/decimal"
)

const (
	serviceTestPrivateKey = "0x4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"
	serviceUserPrivateKey = "0x59c6995e998f97a5a0044976f094538fcb963c90d6f9fbfc7f3a6fcd45d9c25"
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
			return []core.BalanceEntry{
				{Asset: "yusd", Balance: decimal.RequireFromString("7")},
				{Asset: "yellow", Balance: decimal.RequireFromString("7")},
			}, nil
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
		AppSessionID:  "0xsession",
		AppDefinition: definition,
		Version:       1,
		SessionData:   sessionData,
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

	resp, err := service.CreateSession(context.Background(), StoreInitRequest{
		Asset:         "yusd",
		Definition:    rpcDefinition,
		SessionData:   sessionData,
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

func TestCrossLanguagePackSignVectors(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	if !strings.EqualFold(userSigner.Address(), "0x20bcbc2e9f2ee516f2f4881cf8aa0baab49cb072") {
		t.Fatalf("unexpected user signer address = %s", userSigner.Address())
	}
	if !strings.EqualFold(appSigner.Address(), "0x999455567acde6d73e89f6d6c11084f866e33bbb") {
		t.Fatalf("unexpected app signer address = %s", appSigner.Address())
	}

	definition := app.AppDefinitionV1{
		ApplicationID: "store",
		Participants: []app.AppParticipantV1{
			{WalletAddress: userSigner.Address(), SignatureWeight: 1},
			{WalletAddress: appSigner.Address(), SignatureWeight: 1},
		},
		Quorum: 2,
		Nonce:  1_700_000_000_000_000,
	}
	sessionData := `{"intent":"init"}`
	createPayload, err := app.PackCreateAppSessionRequestV1(definition, sessionData)
	if err != nil {
		t.Fatalf("PackCreateAppSessionRequestV1() error = %v", err)
	}
	if got, want := hexutil.Encode(createPayload), "0xa0d3febab481191730b66807bee278eba25d45431be0b408284ae27a7d897b4a"; got != want {
		t.Fatalf("create payload = %s, want TypeScript vector %s", got, want)
	}
	createSig, err := signCreateAppSessionRequest(definition, sessionData, userSigner)
	if err != nil {
		t.Fatalf("signCreateAppSessionRequest() error = %v", err)
	}
	if err := verifyCreateSessionSignature(userSigner.Address(), definition, sessionData, createSig); err != nil {
		t.Fatalf("verifyCreateSessionSignature() error = %v", err)
	}

	update := app.AppStateUpdateV1{
		AppSessionID: "0x0000000000000000000000000000000000000000000000000000000000000009",
		Intent:       app.AppStateUpdateIntentOperate,
		Version:      4,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.1")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.9")},
		},
		SessionData: `{"intent":"purchase","item_id":1,"item_price":"0.9"}`,
	}
	updatePayload, err := app.PackAppStateUpdateV1(update)
	if err != nil {
		t.Fatalf("PackAppStateUpdateV1() error = %v", err)
	}
	if got, want := hexutil.Encode(updatePayload), "0x73fa52d896b17c36e3a4131b9dd609c627c55aac779d616dc0db821d0b75249a"; got != want {
		t.Fatalf("update payload = %s, want TypeScript vector %s", got, want)
	}
	updateSig, err := signAppStateUpdate(update, userSigner)
	if err != nil {
		t.Fatalf("signAppStateUpdate() error = %v", err)
	}
	if err := verifyAppStateSignature(userSigner.Address(), update, updateSig); err != nil {
		t.Fatalf("verifyAppStateSignature() error = %v", err)
	}
}

func TestWalletStoreServiceCreateSessionRejectsWrongSignature(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	client := &testsupport.FakeClient{
		GetAppsFunc: func(context.Context, *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error) {
			return []app.AppInfoV1{{App: app.AppV1{ID: "store"}}}, core.PaginationMetadata{}, nil
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
	service.createAppSessionRPC = func(context.Context, string, rpc.AppSessionsV1CreateAppSessionRequest) (*rpc.AppSessionsV1CreateAppSessionResponse, error) {
		t.Fatal("createAppSessionRPC was called for an invalid signature")
		return nil, nil
	}

	definition := app.AppDefinitionV1{
		ApplicationID: "store",
		Participants: []app.AppParticipantV1{
			{WalletAddress: userSigner.Address(), SignatureWeight: 1},
			{WalletAddress: appSigner.Address(), SignatureWeight: 1},
		},
		Quorum: 2,
		Nonce:  1,
	}
	sessionData := `{"intent":"init"}`
	wrongSig, err := signCreateAppSessionRequest(definition, sessionData, appSigner)
	if err != nil {
		t.Fatalf("signCreateAppSessionRequest() error = %v", err)
	}

	_, err = service.CreateSession(context.Background(), StoreInitRequest{
		Asset: "yusd",
		Definition: rpc.AppDefinitionV1{
			Application: "store",
			Participants: []rpc.AppParticipantV1{
				{WalletAddress: userSigner.Address(), SignatureWeight: 1},
				{WalletAddress: appSigner.Address(), SignatureWeight: 1},
			},
			Quorum: 2,
			Nonce:  "1",
		},
		SessionData:   sessionData,
		UserSignature: wrongSig,
	})
	if err == nil {
		t.Fatal("CreateSession() accepted wrong signature")
	}
	var conflict ConflictError
	if !errors.As(err, &conflict) || conflict.Code() != "invalid_signature" {
		t.Fatalf("CreateSession() error = %#v, want invalid_signature conflict", err)
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
		SessionData: `{"intent":"user_deposit"}`,
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
		SessionData: `{"intent":"user_withdraw","amount":"0.50"}`,
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

	resp, err := service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yusd",
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
	if resp.Bootstrap == nil || resp.Bootstrap.Session.Version != 2 || resp.Bootstrap.Session.UserAllocation != "0.5" {
		t.Fatalf("unexpected response = %#v", resp)
	}
}

func TestWalletStoreServiceSubmitWithdrawAcceptsIntentOnlySessionData(t *testing.T) {
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
		Version:     2,
		SessionData: `{"intent":"user_deposit"}`,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1.0")},
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
		Version:      3,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.5")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
		SessionData: `{"intent":"user_withdraw"}`,
	}
	userSig, err := signAppStateUpdate(update, userSigner)
	if err != nil {
		t.Fatalf("signAppStateUpdate() error = %v", err)
	}
	rpcUpdate := rpc.AppStateUpdateV1{
		AppSessionID: update.AppSessionID,
		Intent:       update.Intent,
		Version:      "3",
		Allocations: []rpc.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: "0.5"},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: "0"},
		},
		SessionData: update.SessionData,
	}

	service.submitAppStateRPC = func(ctx context.Context, wsURL string, req rpc.AppSessionsV1SubmitAppStateRequest) error {
		currentSession.Version = update.Version
		currentSession.SessionData = update.SessionData
		currentSession.Allocations = update.Allocations
		return nil
	}

	resp, err := service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yusd",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	})
	if err != nil {
		t.Fatalf("SubmitUpdate() error = %v", err)
	}
	if resp.Bootstrap == nil || resp.Bootstrap.Session.Version != 3 || resp.Bootstrap.Session.UserAllocation != "0.5" {
		t.Fatalf("unexpected response = %#v", resp)
	}
}

func TestWalletStoreServiceRejectsStaleUpdateVersion(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	now := time.Unix(1_700_000_000, 0).UTC()
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
		Version:     2,
		SessionData: `{"intent":"user_deposit"}`,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1.0")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	service, appStore := newWalletStoreServiceForTest(t, userSigner, appSigner, &currentSession)
	service.now = func() time.Time { return now }

	if err := appStore.UpsertWalletSession(context.Background(), store.WalletStoreSession{
		WalletAddress:  userSigner.Address(),
		Asset:          "yusd",
		AppSessionID:   currentSession.AppSessionID,
		Status:         "open",
		Version:        currentSession.Version,
		UserAllocation: "1",
		AppAllocation:  "0",
		SessionData:    currentSession.SessionData,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWalletSession() error = %v", err)
	}

	update := app.AppStateUpdateV1{
		AppSessionID: currentSession.AppSessionID,
		Intent:       app.AppStateUpdateIntentDeposit,
		Version:      currentSession.Version,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("2.0")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
		SessionData: `{"intent":"user_deposit"}`,
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
			{Participant: userSigner.Address(), Asset: "yusd", Amount: "2.0"},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: "0"},
		},
		SessionData: update.SessionData,
	}

	_, err = service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yusd",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	})
	if err == nil {
		t.Fatal("SubmitUpdate() accepted stale version")
	}
	var conflict ConflictError
	if !errors.As(err, &conflict) || conflict.Code() != "stale_version" {
		t.Fatalf("SubmitUpdate() error = %#v, want stale_version conflict", err)
	}
}

func TestWalletStoreServiceSubmitDepositAcceptsInitialEmptyAllocations(t *testing.T) {
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
		SessionData: `{"intent":"init"}`,
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
		UserAllocation: "0",
		AppAllocation:  "0",
		SessionData:    currentSession.SessionData,
		CreatedAt:      time.Unix(1_700_000_000, 0).UTC(),
		UpdatedAt:      time.Unix(1_700_000_000, 0).UTC(),
	}); err != nil {
		t.Fatalf("UpsertWalletSession() error = %v", err)
	}

	update := app.AppStateUpdateV1{
		AppSessionID: currentSession.AppSessionID,
		Intent:       app.AppStateUpdateIntentDeposit,
		Version:      2,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1.0")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
		SessionData: `{"intent":"user_deposit"}`,
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
			{Participant: userSigner.Address(), Asset: "yusd", Amount: "1.0"},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: "0"},
		},
		SessionData: update.SessionData,
	}

	resp, err := service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yusd",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	})
	if err != nil {
		t.Fatalf("SubmitUpdate() error = %v", err)
	}
	if resp.Status != "signed" || resp.Intent != string(StoreIntentUserDeposit) || resp.AppSignature == "" {
		t.Fatalf("unexpected response = %#v", resp)
	}
	if resp.PendingAction == nil || resp.PendingAction.Type != string(StoreIntentUserDeposit) || resp.PendingAction.Amount != "1" {
		t.Fatalf("unexpected pending deposit action = %#v", resp.PendingAction)
	}
	bootstrap, err := service.Bootstrap(context.Background(), userSigner.Address(), "yusd")
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if bootstrap.PendingAction == nil || bootstrap.PendingAction.AppSignature != resp.AppSignature || bootstrap.PendingAction.AppStateUpdate.Version != "2" {
		t.Fatalf("bootstrap pending action = %#v, want resumable deposit", bootstrap.PendingAction)
	}
}

func TestWalletStoreServiceBootstrapReconcilesDepositCheckpoint(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	sessionData := `{"intent":"user_deposit","amount":"1"}`
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
		Version:     2,
		SessionData: sessionData,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1.0")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	service, appStore := newWalletStoreServiceForTest(t, userSigner, appSigner, &currentSession)
	service.now = func() time.Time { return now }

	if err := appStore.UpsertWalletSession(context.Background(), store.WalletStoreSession{
		WalletAddress:  userSigner.Address(),
		Asset:          "yusd",
		AppSessionID:   currentSession.AppSessionID,
		Status:         "open",
		Version:        1,
		UserAllocation: "0",
		AppAllocation:  "0",
		SessionData:    `{"intent":"init"}`,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWalletSession() error = %v", err)
	}

	update := app.AppStateUpdateV1{
		AppSessionID: currentSession.AppSessionID,
		Intent:       app.AppStateUpdateIntentDeposit,
		Version:      currentSession.Version,
		Allocations:  currentSession.Allocations,
		SessionData:  sessionData,
	}
	userSig, err := signAppStateUpdate(update, userSigner)
	if err != nil {
		t.Fatalf("signAppStateUpdate() error = %v", err)
	}
	appSig, err := signAppStateUpdate(update, appSigner)
	if err != nil {
		t.Fatalf("signAppStateUpdate() app error = %v", err)
	}
	rpcUpdate := rpc.AppStateUpdateV1{
		AppSessionID: update.AppSessionID,
		Intent:       update.Intent,
		Version:      "2",
		Allocations: []rpc.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: "1.0"},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: "0"},
		},
		SessionData: update.SessionData,
	}
	updateJSON, err := json.Marshal(rpcUpdate)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	checkpointID := store.WalletDepositCheckpointID(userSigner.Address(), "yusd")
	if err := appStore.UpsertDepositCheckpoint(context.Background(), store.WalletDepositCheckpoint{
		ID:             checkpointID,
		WalletAddress:  userSigner.Address(),
		Asset:          "yusd",
		AppSessionID:   currentSession.AppSessionID,
		Version:        currentSession.Version,
		Amount:         "1",
		Status:         store.WalletDepositCheckpointStatusAppSigned,
		AppStateUpdate: string(updateJSON),
		UserSignature:  userSig,
		AppSignature:   appSig,
		SessionData:    sessionData,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertDepositCheckpoint() error = %v", err)
	}

	resp, err := service.Bootstrap(context.Background(), userSigner.Address(), "yusd")
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if resp.PendingAction != nil || resp.Session.Version != currentSession.Version || resp.Session.UserAllocation != "1" {
		t.Fatalf("bootstrap = %#v, want reconciled deposit checkpoint", resp)
	}
	if _, err := appStore.GetActiveDepositCheckpoint(context.Background(), userSigner.Address(), "yusd"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("active checkpoint error = %v, want ErrNotFound", err)
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
		SessionData: `{"intent":"user_deposit"}`,
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
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.1")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.9")},
		},
		SessionData: `{"intent":"purchase","item_id":1,"item_price":"0.9"}`,
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
			{Participant: userSigner.Address(), Asset: "yusd", Amount: "0.1"},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: "0.9"},
		},
		SessionData: update.SessionData,
	}

	service.submitAppStateRPC = func(context.Context, string, rpc.AppSessionsV1SubmitAppStateRequest) error {
		currentSession.Version = update.Version
		currentSession.SessionData = update.SessionData
		currentSession.Allocations = update.Allocations
		return nil
	}

	resp, err := service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yusd",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	})
	if err != nil {
		t.Fatalf("SubmitUpdate() error = %v", err)
	}

	if resp.Bootstrap == nil || len(resp.Bootstrap.Library) != 1 || resp.Bootstrap.Library[0].ID != "1" {
		t.Fatalf("unexpected response = %#v", resp)
	}
}

func TestWalletStoreServiceContentUsesPublicSubmittedPurchaseGate(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	now := time.Unix(1_700_000_000, 0).UTC()
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
		Version:     2,
		SessionData: `{"intent":"purchase","item_id":1,"item_price":"0.9"}`,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.1")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.9")},
		},
	}
	service, appStore := newWalletStoreServiceForTest(t, userSigner, appSigner, &currentSession)
	service.now = func() time.Time { return now }

	if err := appStore.UpsertWalletSession(context.Background(), store.WalletStoreSession{
		WalletAddress:  userSigner.Address(),
		Asset:          "yusd",
		AppSessionID:   currentSession.AppSessionID,
		Status:         "open",
		Version:        currentSession.Version,
		UserAllocation: "0.1",
		AppAllocation:  "0.9",
		SessionData:    currentSession.SessionData,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWalletSession() error = %v", err)
	}
	if err := appStore.UpsertPendingWalletPurchase(context.Background(), store.WalletPurchase{
		ID:            store.WalletPurchaseID(userSigner.Address(), "yusd", "1"),
		WalletAddress: userSigner.Address(),
		ItemID:        "1",
		Asset:         "yusd",
		AppSessionID:  currentSession.AppSessionID,
		Version:       currentSession.Version,
		Status:        store.WalletPurchaseStatusPending,
		SessionData:   currentSession.SessionData,
		CreatedAt:     now,
		UpdatedAt:     now,
		PurchasedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertPendingWalletPurchase() error = %v", err)
	}
	if err := appStore.MarkWalletPurchaseSubmitted(context.Background(), store.WalletPurchaseID(userSigner.Address(), "yusd", "1"), now); err != nil {
		t.Fatalf("MarkWalletPurchaseSubmitted() error = %v", err)
	}

	if _, err := service.Content(context.Background(), "1", StoreContentRequest{Asset: "yusd"}); err == nil {
		t.Fatal("Content() without wallet succeeded")
	}

	otherWallet := mustSignerFromKey(t, serviceTestPrivateKey).Address()
	if _, err := service.Content(context.Background(), "1", StoreContentRequest{WalletAddress: otherWallet, Asset: "yusd"}); err == nil {
		t.Fatal("Content() for wallet without store session succeeded")
	}

	item, err := service.Content(context.Background(), "1", StoreContentRequest{WalletAddress: userSigner.Address(), Asset: "yusd"})
	if err != nil {
		t.Fatalf("Content() error = %v", err)
	}
	if item.ID != "1" || item.Content == "" {
		t.Fatalf("unexpected content item = %#v", item)
	}
}

func TestWalletStoreServiceYellowPurchaseAndPublicRead(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	currentSession := app.AppSessionInfoV1{
		AppSessionID: "0xyellowsession",
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
		SessionData: `{"intent":"user_deposit"}`,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yellow", Amount: decimal.RequireFromString("2.0")},
			{Participant: appSigner.Address(), Asset: "yellow", Amount: decimal.Zero},
		},
	}
	service, appStore := newWalletStoreServiceForTest(t, userSigner, appSigner, &currentSession)
	service.now = func() time.Time { return now }

	if err := appStore.UpsertWalletSession(context.Background(), store.WalletStoreSession{
		WalletAddress:  userSigner.Address(),
		Asset:          "yellow",
		AppSessionID:   currentSession.AppSessionID,
		Status:         "open",
		Version:        currentSession.Version,
		UserAllocation: "2.0",
		AppAllocation:  "0",
		SessionData:    currentSession.SessionData,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWalletSession() error = %v", err)
	}

	update := app.AppStateUpdateV1{
		AppSessionID: currentSession.AppSessionID,
		Intent:       app.AppStateUpdateIntentOperate,
		Version:      2,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yellow", Amount: decimal.RequireFromString("0.65")},
			{Participant: appSigner.Address(), Asset: "yellow", Amount: decimal.RequireFromString("1.35")},
		},
		SessionData: `{"intent":"purchase","item_id":1,"item_price":"1.35"}`,
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
			{Participant: userSigner.Address(), Asset: "yellow", Amount: "0.65"},
			{Participant: appSigner.Address(), Asset: "yellow", Amount: "1.35"},
		},
		SessionData: update.SessionData,
	}

	service.submitAppStateRPC = func(context.Context, string, rpc.AppSessionsV1SubmitAppStateRequest) error {
		currentSession.Version = update.Version
		currentSession.SessionData = update.SessionData
		currentSession.Allocations = update.Allocations
		return nil
	}

	resp, err := service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yellow",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	})
	if err != nil {
		t.Fatalf("SubmitUpdate() yellow error = %v", err)
	}
	if resp.Bootstrap == nil || len(resp.Bootstrap.Library) != 1 || resp.Bootstrap.Library[0].Price != "1.35" {
		t.Fatalf("unexpected yellow purchase response = %#v", resp)
	}

	item, err := service.Content(context.Background(), "1", StoreContentRequest{WalletAddress: userSigner.Address(), Asset: "yellow"})
	if err != nil {
		t.Fatalf("Content() yellow error = %v", err)
	}
	if item.Prices["yellow"] != "1.35" || item.Content == "" {
		t.Fatalf("unexpected yellow content item = %#v", item)
	}
}

func TestWalletStoreServiceContentRejectsUnpurchasedItem(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	now := time.Unix(1_700_000_000, 0).UTC()
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
		SessionData: `{"intent":"init"}`,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1.0")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	service, appStore := newWalletStoreServiceForTest(t, userSigner, appSigner, &currentSession)
	service.now = func() time.Time { return now }

	if err := appStore.UpsertWalletSession(context.Background(), store.WalletStoreSession{
		WalletAddress:  userSigner.Address(),
		Asset:          "yusd",
		AppSessionID:   currentSession.AppSessionID,
		Status:         "open",
		Version:        currentSession.Version,
		UserAllocation: "1",
		AppAllocation:  "0",
		SessionData:    currentSession.SessionData,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWalletSession() error = %v", err)
	}

	if _, err := service.Content(context.Background(), "1", StoreContentRequest{WalletAddress: userSigner.Address(), Asset: "yusd"}); err == nil {
		t.Fatal("Content() succeeded for unpurchased item")
	}
}

func TestWalletStoreServiceBootstrapReconcilesPendingPurchase(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	sessionData := `{"intent":"purchase","item_id":1,"item_price":"0.9"}`
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
		Version:     2,
		SessionData: sessionData,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.1")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.9")},
		},
	}
	service, appStore := newWalletStoreServiceForTest(t, userSigner, appSigner, &currentSession)
	service.now = func() time.Time { return now }

	if err := appStore.UpsertWalletSession(context.Background(), store.WalletStoreSession{
		WalletAddress:  userSigner.Address(),
		Asset:          "yusd",
		AppSessionID:   currentSession.AppSessionID,
		Status:         "open",
		Version:        1,
		UserAllocation: "1",
		AppAllocation:  "0",
		SessionData:    `{"intent":"user_deposit"}`,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWalletSession() error = %v", err)
	}
	if err := appStore.UpsertPendingWalletPurchase(context.Background(), store.WalletPurchase{
		ID:            store.WalletPurchaseID(userSigner.Address(), "yusd", "1"),
		WalletAddress: userSigner.Address(),
		ItemID:        "1",
		Asset:         "yusd",
		AppSessionID:  currentSession.AppSessionID,
		Version:       currentSession.Version,
		Status:        store.WalletPurchaseStatusPending,
		SessionData:   sessionData,
		CreatedAt:     now,
		UpdatedAt:     now,
		PurchasedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertPendingWalletPurchase() error = %v", err)
	}

	resp, err := service.Bootstrap(context.Background(), userSigner.Address(), "yusd")
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if len(resp.Library) != 1 || resp.Library[0].ID != "1" {
		t.Fatalf("library = %#v, want reconciled item 1", resp.Library)
	}
}

func TestWalletStoreServiceSubmitPurchaseFailureDoesNotUnlockAndCanRetry(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	now := time.Unix(1_700_000_000, 0).UTC()
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
		SessionData: `{"intent":"user_deposit"}`,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1.0")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	service, appStore := newWalletStoreServiceForTest(t, userSigner, appSigner, &currentSession)
	service.now = func() time.Time { return now }

	if err := appStore.UpsertWalletSession(context.Background(), store.WalletStoreSession{
		WalletAddress:  userSigner.Address(),
		Asset:          "yusd",
		AppSessionID:   currentSession.AppSessionID,
		Status:         "open",
		Version:        currentSession.Version,
		UserAllocation: "1",
		AppAllocation:  "0",
		SessionData:    currentSession.SessionData,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWalletSession() error = %v", err)
	}

	update, rpcUpdate, userSig := signedPurchaseUpdate(t, userSigner, appSigner, currentSession)
	service.submitAppStateRPC = func(context.Context, string, rpc.AppSessionsV1SubmitAppStateRequest) error {
		return errors.New("clearnode down")
	}

	if _, err := service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yusd",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	}); err == nil {
		t.Fatal("SubmitUpdate() succeeded despite Clearnode error")
	}
	owned, err := appStore.HasWalletPurchase(context.Background(), userSigner.Address(), "1", "yusd")
	if err != nil {
		t.Fatalf("HasWalletPurchase() error = %v", err)
	}
	if owned {
		t.Fatal("failed purchase was unlocked")
	}

	service.submitAppStateRPC = func(context.Context, string, rpc.AppSessionsV1SubmitAppStateRequest) error {
		currentSession.Version = update.Version
		currentSession.SessionData = update.SessionData
		currentSession.Allocations = update.Allocations
		return nil
	}
	resp, err := service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yusd",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	})
	if err != nil {
		t.Fatalf("SubmitUpdate() retry error = %v", err)
	}
	if resp.Bootstrap == nil || len(resp.Bootstrap.Library) != 1 {
		t.Fatalf("retry did not unlock purchase: %#v", resp)
	}
}

func TestWalletStoreServiceDuplicateSubmittedPurchaseRejected(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	now := time.Unix(1_700_000_000, 0).UTC()
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
		SessionData: `{"intent":"user_deposit"}`,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1.0")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	service, appStore := newWalletStoreServiceForTest(t, userSigner, appSigner, &currentSession)
	service.now = func() time.Time { return now }

	if err := appStore.UpsertWalletSession(context.Background(), store.WalletStoreSession{
		WalletAddress:  userSigner.Address(),
		Asset:          "yusd",
		AppSessionID:   currentSession.AppSessionID,
		Status:         "open",
		Version:        currentSession.Version,
		UserAllocation: "1",
		AppAllocation:  "0",
		SessionData:    currentSession.SessionData,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWalletSession() error = %v", err)
	}
	if err := appStore.UpsertPendingWalletPurchase(context.Background(), store.WalletPurchase{
		ID:            store.WalletPurchaseID(userSigner.Address(), "yusd", "1"),
		WalletAddress: userSigner.Address(),
		ItemID:        "1",
		Asset:         "yusd",
		AppSessionID:  currentSession.AppSessionID,
		Version:       2,
		Status:        store.WalletPurchaseStatusPending,
		SessionData:   `{"intent":"purchase","item_id":1,"item_price":"0.9"}`,
		CreatedAt:     now,
		UpdatedAt:     now,
		PurchasedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertPendingWalletPurchase() error = %v", err)
	}
	if err := appStore.MarkWalletPurchaseSubmitted(context.Background(), store.WalletPurchaseID(userSigner.Address(), "yusd", "1"), now); err != nil {
		t.Fatalf("MarkWalletPurchaseSubmitted() error = %v", err)
	}

	_, rpcUpdate, userSig := signedPurchaseUpdate(t, userSigner, appSigner, currentSession)
	if _, err := service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yusd",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	}); err == nil {
		t.Fatal("SubmitUpdate() duplicate purchase succeeded")
	} else {
		var conflict ConflictError
		if !errors.As(err, &conflict) || conflict.Code() != "duplicate_purchase" {
			t.Fatalf("SubmitUpdate() error = %#v, want duplicate_purchase conflict", err)
		}
	}
}

func TestStoreCatalogIncludesYUSDAndYellowItems(t *testing.T) {
	t.Parallel()

	service := NewWalletStoreService(nil, nil, mustStoreAppSigner(t), "Nitrolite App Session Store", "store", map[string]uint64{
		"yusd":   11155111,
		"yellow": 11155111,
		"other":  11155111,
	}, "wss://example.invalid")

	if !reflect.DeepEqual(service.supportedAssets, []string{"yusd", "yellow"}) {
		t.Fatalf("supportedAssets = %#v, want yusd/yellow", service.supportedAssets)
	}
	if len(service.catalog) != 5 {
		t.Fatalf("catalog length = %d, want 5", len(service.catalog))
	}
	if service.catalog[0].Prices["yusd"] != "0.9" {
		t.Fatalf("item 1 yusd price = %q, want 0.9", service.catalog[0].Prices["yusd"])
	}
	for _, item := range service.catalog {
		if item.Prices["yusd"] == "" || item.Prices["yellow"] == "" {
			t.Fatalf("catalog item missing yusd/yellow price: %#v", item)
		}
	}
}

func mustTestSigner(t *testing.T) appsigning.Signer {
	t.Helper()

	return mustSignerFromKey(t, serviceUserPrivateKey)
}

func mustSignerFromKey(t *testing.T, privateKey string) appsigning.Signer {
	t.Helper()

	signer, err := appsigning.NewStoreAppSigner("", privateKey)
	if err != nil {
		t.Fatalf("NewStoreAppSigner() error = %v", err)
	}
	return signer
}

func mustStoreAppSigner(t *testing.T) appsigning.Signer {
	t.Helper()

	signer, err := appsigning.NewStoreAppSigner("", serviceTestPrivateKey)
	if err != nil {
		t.Fatalf("NewStoreAppSigner() error = %v", err)
	}
	return signer
}

func newWalletStoreServiceForTest(t *testing.T, userSigner appsigning.Signer, appSigner appsigning.Signer, currentSession *app.AppSessionInfoV1) (*WalletStoreService, *store.Store) {
	t.Helper()

	client := &testsupport.FakeClient{
		GetBalancesFunc: func(context.Context, string) ([]core.BalanceEntry, error) {
			return []core.BalanceEntry{{Asset: "yusd", Balance: decimal.RequireFromString("7")}}, nil
		},
		GetAppSessionsFunc: func(context.Context, *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
			if currentSession == nil {
				return nil, core.PaginationMetadata{}, nil
			}
			return []app.AppSessionInfoV1{*currentSession}, core.PaginationMetadata{}, nil
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

	service := NewWalletStoreService(manager, appStore, appSigner, "Nitrolite App Session Store", "store", map[string]uint64{"yusd": 11155111, "yellow": 11155111}, "wss://example.invalid")
	_ = userSigner
	return service, appStore
}

func signedPurchaseUpdate(t *testing.T, userSigner appsigning.Signer, appSigner appsigning.Signer, currentSession app.AppSessionInfoV1) (app.AppStateUpdateV1, rpc.AppStateUpdateV1, string) {
	t.Helper()

	update := app.AppStateUpdateV1{
		AppSessionID: currentSession.AppSessionID,
		Intent:       app.AppStateUpdateIntentOperate,
		Version:      currentSession.Version + 1,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.1")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.9")},
		},
		SessionData: `{"intent":"purchase","item_id":1,"item_price":"0.9"}`,
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
			{Participant: userSigner.Address(), Asset: "yusd", Amount: "0.1"},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: "0.9"},
		},
		SessionData: update.SessionData,
	}
	return update, rpcUpdate, userSig
}
