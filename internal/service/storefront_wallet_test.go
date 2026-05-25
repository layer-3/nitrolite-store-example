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
	"github.com/layer-3/nitrolite/pkg/sign"
	sdk "github.com/layer-3/nitrolite/sdk/go"
	"github.com/shopspring/decimal"
)

const (
	serviceTestPrivateKey = "0x4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"
	serviceUserPrivateKey = "0x59c6995e998f97a5a0044976f094538fcb963c90d6f9fbfc7f3a6fcd45d9c25"
)

func fundedHomeState(wallet string, asset string, balance string) *core.State {
	homeChannelID := "0xhome"
	return &core.State{
		ID:            "0xstate",
		Asset:         asset,
		UserWallet:    wallet,
		HomeChannelID: &homeChannelID,
		HomeLedger: core.Ledger{
			UserBalance: decimal.RequireFromString(balance),
			UserNetFlow: decimal.RequireFromString(balance),
			NodeBalance: decimal.Zero,
			NodeNetFlow: decimal.Zero,
		},
	}
}

func receivedOffchainState(wallet string, asset string, balance string) *core.State {
	return &core.State{
		ID:         "0xreceived",
		Asset:      asset,
		UserWallet: wallet,
		HomeLedger: core.Ledger{
			UserBalance: decimal.RequireFromString(balance),
			UserNetFlow: decimal.RequireFromString(balance),
			NodeBalance: decimal.Zero,
			NodeNetFlow: decimal.Zero,
		},
	}
}

func TestWalletStoreServiceCreateSessionForwardsExactPayload(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	client := &testsupport.FakeClient{
		GetAppsFunc: func(context.Context, *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error) {
			return []app.AppInfoV1{{App: app.AppV1{ID: "store"}}}, core.PaginationMetadata{}, nil
		},
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, _ bool) (*core.State, error) {
			return fundedHomeState(wallet, asset, "7"), nil
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

func TestWalletStoreServiceCreateSessionRequiresFundedHomeChannel(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	client := &testsupport.FakeClient{
		GetAppsFunc: func(context.Context, *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error) {
			return []app.AppInfoV1{{App: app.AppV1{ID: "store"}}}, core.PaginationMetadata{}, nil
		},
		GetLatestStateFunc: func(context.Context, string, string, bool) (*core.State, error) {
			return nil, nil
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
		t.Fatal("createAppSessionRPC must not be called without a funded home channel")
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
	userSig, err := signCreateAppSessionRequest(definition, sessionData, userSigner)
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
		UserSignature: userSig,
	})
	if err == nil {
		t.Fatal("CreateSession() accepted a wallet without a funded home channel")
	}
	var conflict ConflictError
	if !errors.As(err, &conflict) || conflict.Code() != "channel_funds_required" {
		t.Fatalf("CreateSession() error = %#v, want channel_funds_required conflict", err)
	}
}

func TestWalletStoreServiceBootstrapReportsAckRequiredChannel(t *testing.T) {
	t.Parallel()

	appSigner := mustStoreAppSigner(t)
	client := &testsupport.FakeClient{
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, signed bool) (*core.State, error) {
			if signed {
				return nil, errors.New("signed state not found")
			}
			return fundedHomeState(wallet, asset, "5"), nil
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
	resp, err := service.Bootstrap(context.Background(), "0x1111111111111111111111111111111111111111", "yusd")
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if resp.ChannelReadiness.Status != "ack_required" {
		t.Fatalf("channel_readiness.status = %q, want ack_required", resp.ChannelReadiness.Status)
	}
	if resp.AvailableBalance != "0" {
		t.Fatalf("available_balance = %q, want 0", resp.AvailableBalance)
	}
	if resp.ChannelReadiness.PendingBalance != "5" {
		t.Fatalf("pending_balance = %q, want 5", resp.ChannelReadiness.PendingBalance)
	}
	if resp.ChannelReadiness.RequiresChannelCreation {
		t.Fatal("requires_channel_creation = true, want false")
	}
}

func TestWalletStoreServiceBootstrapIncludesAssetDecimals(t *testing.T) {
	t.Parallel()

	appSigner := mustStoreAppSigner(t)
	client := &testsupport.FakeClient{
		GetAssetsFunc: func(context.Context, *uint64) ([]core.Asset, error) {
			return []core.Asset{
				{Symbol: "YUSD", Decimals: 6},
				{Symbol: "YELLOW", Decimals: 18},
			}, nil
		},
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, _ bool) (*core.State, error) {
			return fundedHomeState(wallet, asset, "5"), nil
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
	resp, err := service.Bootstrap(context.Background(), "0x1111111111111111111111111111111111111111", "yusd")
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if resp.AssetDecimals["yusd"] != 6 {
		t.Fatalf("asset_decimals[yusd] = %d, want 6", resp.AssetDecimals["yusd"])
	}
	if resp.AssetDecimals["yellow"] != 18 {
		t.Fatalf("asset_decimals[yellow] = %d, want 18", resp.AssetDecimals["yellow"])
	}
}

func TestWalletStoreServiceBootstrapReportsAckRequiredForReceivedFundsWithoutHomeChannel(t *testing.T) {
	t.Parallel()

	appSigner := mustStoreAppSigner(t)
	client := &testsupport.FakeClient{
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, signed bool) (*core.State, error) {
			if signed {
				return nil, errors.New("signed state not found")
			}
			return receivedOffchainState(wallet, asset, "5"), nil
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
	resp, err := service.Bootstrap(context.Background(), "0x1111111111111111111111111111111111111111", "yusd")
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if resp.ChannelReadiness.Status != "ack_required" {
		t.Fatalf("channel_readiness.status = %q, want ack_required", resp.ChannelReadiness.Status)
	}
	if !resp.ChannelReadiness.RequiresChannelCreation {
		t.Fatal("requires_channel_creation = false, want true")
	}
	if resp.AvailableBalance != "0" {
		t.Fatalf("available_balance = %q, want 0", resp.AvailableBalance)
	}
	if resp.ChannelReadiness.PendingBalance != "5" {
		t.Fatalf("pending_balance = %q, want 5", resp.ChannelReadiness.PendingBalance)
	}
}

func TestWalletStoreServiceBootstrapKeepsSignedNoHomeChannelStateInSetup(t *testing.T) {
	t.Parallel()

	appSigner := mustStoreAppSigner(t)
	userSig := "0xsigned"
	client := &testsupport.FakeClient{
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, _ bool) (*core.State, error) {
			state := receivedOffchainState(wallet, asset, "5")
			state.UserSig = &userSig
			return state, nil
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
	resp, err := service.Bootstrap(context.Background(), "0x1111111111111111111111111111111111111111", "yusd")
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if resp.ChannelReadiness.Status != "ack_required" {
		t.Fatalf("channel_readiness.status = %q, want ack_required", resp.ChannelReadiness.Status)
	}
	if !resp.ChannelReadiness.RequiresChannelCreation {
		t.Fatal("requires_channel_creation = false, want true")
	}
	if resp.AvailableBalance != "0" {
		t.Fatalf("available_balance = %q, want 0", resp.AvailableBalance)
	}
}

func TestWalletStoreServiceBootstrapReportsAckRequiredWhenPendingReleaseIsNewer(t *testing.T) {
	t.Parallel()

	appSigner := mustStoreAppSigner(t)
	userSig := "0xsigned"
	client := &testsupport.FakeClient{
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, signed bool) (*core.State, error) {
			if signed {
				state := fundedHomeState(wallet, asset, "0.5")
				state.ID = "0xsignedstate"
				state.Version = 4
				state.UserSig = &userSig
				return state, nil
			}
			state := fundedHomeState(wallet, asset, "10")
			state.ID = "0xpendingrelease"
			state.Version = 5
			state.Transition.Type = core.TransitionTypeRelease
			state.Transition.Amount = decimal.RequireFromString("9.5")
			return state, nil
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

	service := NewWalletStoreService(manager, appStore, appSigner, "Nitrolite App Session Store", "store", map[string]uint64{"yellow": 11155111}, "wss://example.invalid")
	resp, err := service.Bootstrap(context.Background(), "0x1111111111111111111111111111111111111111", "yellow")
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if resp.ChannelReadiness.Status != "ack_required" {
		t.Fatalf("channel_readiness.status = %q, want ack_required", resp.ChannelReadiness.Status)
	}
	if resp.AvailableBalance != "0.5" {
		t.Fatalf("available_balance = %q, want 0.5", resp.AvailableBalance)
	}
	if resp.ChannelReadiness.PendingBalance != "10" {
		t.Fatalf("pending_balance = %q, want 10", resp.ChannelReadiness.PendingBalance)
	}
	if resp.ChannelReadiness.PendingTransition != "release" {
		t.Fatalf("pending_transition = %q, want release", resp.ChannelReadiness.PendingTransition)
	}
	if resp.ChannelReadiness.PendingAmount != "9.5" {
		t.Fatalf("pending_amount = %q, want 9.5", resp.ChannelReadiness.PendingAmount)
	}
	if resp.ChannelReadiness.RequiresChannelCreation {
		t.Fatal("requires_channel_creation = true, want false")
	}
}

func TestWalletStoreServiceBootstrapReportsDepositRequiredChannel(t *testing.T) {
	t.Parallel()

	appSigner := mustStoreAppSigner(t)
	client := &testsupport.FakeClient{
		GetLatestStateFunc: func(context.Context, string, string, bool) (*core.State, error) {
			return nil, errors.New("state not found")
		},
		GetOnChainBalanceFunc: func(_ context.Context, chainID uint64, asset string, wallet string) (decimal.Decimal, error) {
			if chainID != 11155111 {
				t.Fatalf("chainID = %d, want 11155111", chainID)
			}
			if asset != "yellow" {
				t.Fatalf("asset = %q, want yellow", asset)
			}
			if wallet != "0x1111111111111111111111111111111111111111" {
				t.Fatalf("wallet = %q, want test wallet", wallet)
			}
			return decimal.RequireFromString("3"), nil
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

	service := NewWalletStoreService(
		manager,
		appStore,
		appSigner,
		"Nitrolite App Session Store",
		"store",
		map[string]uint64{"yellow": 11155111, "yusd": 11155111},
		"wss://example.invalid",
		map[string]string{"yellow": "12"},
	)
	resp, err := service.Bootstrap(context.Background(), "0x1111111111111111111111111111111111111111", "yellow")
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if resp.ChannelReadiness.Status != "deposit_required" {
		t.Fatalf("channel_readiness.status = %q, want deposit_required", resp.ChannelReadiness.Status)
	}
	if resp.ChannelReadiness.OnChainBalance != "3" {
		t.Fatalf("on_chain_balance = %q, want 3", resp.ChannelReadiness.OnChainBalance)
	}
	if resp.ChannelReadiness.BootstrapAmount != "12" {
		t.Fatalf("bootstrap_amount = %q, want 12", resp.ChannelReadiness.BootstrapAmount)
	}
	if resp.ChannelReadiness.RequiresChannelCreation {
		t.Fatal("requires_channel_creation = true, want false")
	}
}

func TestWalletStoreServiceEnsureAppSkipsDisabledRegistry(t *testing.T) {
	t.Parallel()

	appSigner := mustStoreAppSigner(t)
	client := &testsupport.FakeClient{
		GetAppsFunc: func(context.Context, *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error) {
			return nil, core.PaginationMetadata{}, errors.New("rpc returned error: apps.v1 group is disabled")
		},
		RegisterAppFunc: func(context.Context, string, string, bool) error {
			t.Fatal("RegisterApp must not be called when apps.v1 is disabled")
			return nil
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
	if err := service.ensureApp(context.Background()); err != nil {
		t.Fatalf("ensureApp() error = %v, want nil when apps.v1 is disabled", err)
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
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, _ bool) (*core.State, error) {
			return fundedHomeState(wallet, asset, "7"), nil
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

	expectedRPCUpdate := rpcUpdate
	expectedRPCUpdate.Allocations = []rpc.AppAllocationV1{
		{Participant: userSigner.Address(), Asset: "yusd", Amount: "0.500000"},
		{Participant: appSigner.Address(), Asset: "yusd", Amount: "0.000000"},
	}
	if !reflect.DeepEqual(captured.AppStateUpdate, expectedRPCUpdate) {
		t.Fatalf("app_state_update mismatch\n got: %#v\nwant: %#v", captured.AppStateUpdate, expectedRPCUpdate)
	}
	if len(captured.QuorumSigs) != 2 || captured.QuorumSigs[0] != userSig || captured.QuorumSigs[1] == "" {
		t.Fatalf("unexpected quorum signatures = %#v", captured.QuorumSigs)
	}
	if resp.Bootstrap == nil || resp.Bootstrap.Session.Version != 2 || resp.Bootstrap.Session.UserAllocation != "0.5" {
		t.Fatalf("unexpected response = %#v", resp)
	}
}

func TestWalletStoreServiceSubmitWithdrawNormalizesYUSDTrailingScaleBeforeNitronode(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
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
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	service, appStore := newWalletStoreServiceForTest(t, userSigner, appSigner, &currentSession)
	now := time.Unix(1_700_000_000, 0).UTC()
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
		Intent:       app.AppStateUpdateIntentWithdraw,
		Version:      2,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.100000000000000000")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
		SessionData: `{"intent":"user_withdraw","amount":"0.900000000000000000"}`,
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
			{Participant: userSigner.Address(), Asset: "yusd", Amount: "0.100000000000000000"},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: "0.000000000000000000"},
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

	if _, err := service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yusd",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	}); err != nil {
		t.Fatalf("SubmitUpdate() error = %v", err)
	}

	if got := captured.AppStateUpdate.Allocations[0].Amount; got != "0.100000" {
		t.Fatalf("captured wallet amount = %q, want 0.100000", got)
	}
	if got := captured.AppStateUpdate.Allocations[1].Amount; got != "0.000000" {
		t.Fatalf("captured app amount = %q, want 0.000000", got)
	}
}

func TestWalletStoreServiceSubmitWithdrawRejectsRealYUSDOverPrecision(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
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
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	service, appStore := newWalletStoreServiceForTest(t, userSigner, appSigner, &currentSession)
	now := time.Unix(1_700_000_000, 0).UTC()
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
		Intent:       app.AppStateUpdateIntentWithdraw,
		Version:      2,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.1000001")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
		SessionData: `{"intent":"user_withdraw","amount":"0.8999999"}`,
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
			{Participant: userSigner.Address(), Asset: "yusd", Amount: "0.1000001"},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: "0"},
		},
		SessionData: update.SessionData,
	}

	service.submitAppStateRPC = func(context.Context, string, rpc.AppSessionsV1SubmitAppStateRequest) error {
		t.Fatal("submitAppStateRPC must not be called for over-precision amounts")
		return nil
	}

	_, err = service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yusd",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	})
	if err == nil {
		t.Fatal("SubmitUpdate() accepted over-precision yusd amount")
	}
	var validation ValidationError
	if !errors.As(err, &validation) || !strings.Contains(validation.Error(), "YUSD supports up to 6 decimals") {
		t.Fatalf("SubmitUpdate() error = %#v, want yusd precision validation", err)
	}
}

func TestWalletStoreServiceSubmitWithdrawAcceptsYellowEighteenDecimals(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
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
			{Participant: userSigner.Address(), Asset: "yellow", Amount: decimal.RequireFromString("1")},
			{Participant: appSigner.Address(), Asset: "yellow", Amount: decimal.Zero},
		},
	}
	service, appStore := newWalletStoreServiceForTest(t, userSigner, appSigner, &currentSession)
	now := time.Unix(1_700_000_000, 0).UTC()
	service.now = func() time.Time { return now }
	if err := appStore.UpsertWalletSession(context.Background(), store.WalletStoreSession{
		WalletAddress:  userSigner.Address(),
		Asset:          "yellow",
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
		Intent:       app.AppStateUpdateIntentWithdraw,
		Version:      2,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yellow", Amount: decimal.RequireFromString("0.100000000000000001")},
			{Participant: appSigner.Address(), Asset: "yellow", Amount: decimal.Zero},
		},
		SessionData: `{"intent":"user_withdraw","amount":"0.899999999999999999"}`,
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
			{Participant: userSigner.Address(), Asset: "yellow", Amount: "0.100000000000000001"},
			{Participant: appSigner.Address(), Asset: "yellow", Amount: "0"},
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

	if _, err := service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yellow",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	}); err != nil {
		t.Fatalf("SubmitUpdate() error = %v", err)
	}
	if got := captured.AppStateUpdate.Allocations[0].Amount; got != "0.100000000000000001" {
		t.Fatalf("captured wallet amount = %q, want 0.100000000000000001", got)
	}
}

func TestWalletStoreServiceSubmitWithdrawAcceptsIntentOnlySessionData(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	appSigner := mustStoreAppSigner(t)
	client := &testsupport.FakeClient{
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, _ bool) (*core.State, error) {
			return fundedHomeState(wallet, asset, "7"), nil
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

func TestWalletStoreServiceSubmitAppStateAcceptsAuthorizedSessionKey(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	sessionKeySigner := mustEnvSigner(t, "0x8b3a350cf5c34c9194ca3a545d7f46d8eeb98c5fe8e4336cca048a0fbb8e4b3f")
	appSigner := mustStoreAppSigner(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	currentSession := app.AppSessionInfoV1{
		AppSessionID: "0x00000000000000000000000000000000000000000000000000000000000000aa",
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
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	client := &testsupport.FakeClient{
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, _ bool) (*core.State, error) {
			return fundedHomeState(wallet, asset, "7"), nil
		},
		GetAppSessionsFunc: func(context.Context, *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
			return []app.AppSessionInfoV1{currentSession}, core.PaginationMetadata{}, nil
		},
		GetLastAppKeyStatesFunc: func(_ context.Context, userAddress string, opts *sdk.GetLastKeyStatesOptions) ([]app.AppSessionKeyStateV1, error) {
			if !strings.EqualFold(userAddress, userSigner.Address()) {
				t.Fatalf("session key lookup userAddress = %q, want %q", userAddress, userSigner.Address())
			}
			if opts == nil || opts.SessionKey == nil || !strings.EqualFold(*opts.SessionKey, sessionKeySigner.Address()) {
				t.Fatalf("session key lookup opts = %#v, want session key %s", opts, sessionKeySigner.Address())
			}
			return []app.AppSessionKeyStateV1{{
				UserAddress:    userSigner.Address(),
				SessionKey:     sessionKeySigner.Address(),
				Version:        1,
				AppSessionIDs:  []string{currentSession.AppSessionID},
				ApplicationIDs: []string{},
				ExpiresAt:      now.Add(time.Hour),
				UserSig:        "0xusersig",
			}}, nil
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
		Intent:       app.AppStateUpdateIntentWithdraw,
		Version:      2,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("0.5")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
		SessionData: `{"intent":"user_withdraw","amount":"0.500000"}`,
	}
	userSig, err := signAppStateUpdateWithSessionKey(update, sessionKeySigner)
	if err != nil {
		t.Fatalf("signAppStateUpdateWithSessionKey() error = %v", err)
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
	if len(captured.QuorumSigs) != 2 || captured.QuorumSigs[0] != userSig || captured.QuorumSigs[1] == "" {
		t.Fatalf("unexpected quorum signatures = %#v", captured.QuorumSigs)
	}
	if resp.Bootstrap == nil || resp.Bootstrap.Session.Version != 2 {
		t.Fatalf("unexpected response = %#v", resp)
	}
}

func TestWalletStoreServiceSubmitAppStateRejectsSessionKeyOutsideSession(t *testing.T) {
	t.Parallel()

	userSigner := mustTestSigner(t)
	sessionKeySigner := mustEnvSigner(t, "0x2191ef87e392377ec08e7c08eb105ef5448eced5f4067f7607e1de4cc3bf8ae6")
	appSigner := mustStoreAppSigner(t)
	currentSession := app.AppSessionInfoV1{
		AppSessionID: "0x00000000000000000000000000000000000000000000000000000000000000bb",
		AppDefinition: app.AppDefinitionV1{
			ApplicationID: "store",
			Participants: []app.AppParticipantV1{
				{WalletAddress: userSigner.Address(), SignatureWeight: 1},
				{WalletAddress: appSigner.Address(), SignatureWeight: 1},
			},
			Quorum: 2,
			Nonce:  1,
		},
		Version: 1,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	client := &testsupport.FakeClient{
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, _ bool) (*core.State, error) {
			return fundedHomeState(wallet, asset, "7"), nil
		},
		GetAppSessionsFunc: func(context.Context, *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
			return []app.AppSessionInfoV1{currentSession}, core.PaginationMetadata{}, nil
		},
		GetLastAppKeyStatesFunc: func(context.Context, string, *sdk.GetLastKeyStatesOptions) ([]app.AppSessionKeyStateV1, error) {
			return []app.AppSessionKeyStateV1{{
				UserAddress:    userSigner.Address(),
				SessionKey:     sessionKeySigner.Address(),
				Version:        1,
				AppSessionIDs:  []string{"0x00000000000000000000000000000000000000000000000000000000000000cc"},
				ApplicationIDs: []string{},
				ExpiresAt:      time.Now().Add(time.Hour),
				UserSig:        "0xusersig",
			}}, nil
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
	if err := appStore.UpsertWalletSession(context.Background(), store.WalletStoreSession{
		WalletAddress:  userSigner.Address(),
		Asset:          "yusd",
		AppSessionID:   currentSession.AppSessionID,
		Status:         "open",
		Version:        currentSession.Version,
		UserAllocation: "1",
		AppAllocation:  "0",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
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
		SessionData: `{"intent":"user_withdraw","amount":"0.500000"}`,
	}
	userSig, err := signAppStateUpdateWithSessionKey(update, sessionKeySigner)
	if err != nil {
		t.Fatalf("signAppStateUpdateWithSessionKey() error = %v", err)
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

	service.submitAppStateRPC = func(context.Context, string, rpc.AppSessionsV1SubmitAppStateRequest) error {
		t.Fatal("submitAppStateRPC must not be called for unauthorized session key")
		return nil
	}

	_, err = service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yusd",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	})
	if err == nil {
		t.Fatal("SubmitUpdate() accepted session key outside current app session")
	}
	var conflict ConflictError
	if !errors.As(err, &conflict) || conflict.Code() != "invalid_signature" {
		t.Fatalf("SubmitUpdate() error = %#v, want invalid_signature conflict", err)
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
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, _ bool) (*core.State, error) {
			return fundedHomeState(wallet, asset, "7"), nil
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

func TestWalletStoreServiceSubmitDepositRejectsAmountAboveAvailableBalance(t *testing.T) {
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
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	client := &testsupport.FakeClient{
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, _ bool) (*core.State, error) {
			return fundedHomeState(wallet, asset, "0.5"), nil
		},
		GetAppSessionsFunc: func(context.Context, *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
			return []app.AppSessionInfoV1{currentSession}, core.PaginationMetadata{}, nil
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
	service.now = func() time.Time { return now }
	if err := appStore.UpsertWalletSession(context.Background(), store.WalletStoreSession{
		WalletAddress:  userSigner.Address(),
		Asset:          "yusd",
		AppSessionID:   currentSession.AppSessionID,
		Status:         "open",
		Version:        currentSession.Version,
		UserAllocation: "0",
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
		Version:      2,
		Allocations: []app.AppAllocationV1{
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.RequireFromString("1.0")},
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
		SessionData: `{"intent":"user_deposit","amount":"1"}`,
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

	_, err = service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yusd",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	})
	if err == nil {
		t.Fatal("SubmitUpdate() accepted over-balance deposit")
	}
	var conflict ConflictError
	if !errors.As(err, &conflict) || conflict.Code() != "insufficient_balance" {
		t.Fatalf("SubmitUpdate() error = %#v, want insufficient_balance conflict", err)
	}
	if _, err := appStore.GetActiveDepositCheckpoint(context.Background(), userSigner.Address(), "yusd"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("active checkpoint error = %v, want ErrNotFound", err)
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
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, _ bool) (*core.State, error) {
			return fundedHomeState(wallet, asset, "7"), nil
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
		return errors.New("nitronode rejected stale app version")
	}

	if _, err := service.SubmitUpdate(context.Background(), StoreUpdateRequest{
		Asset:          "yusd",
		AppStateUpdate: &rpcUpdate,
		UserSignature:  userSig,
	}); err == nil {
		t.Fatal("SubmitUpdate() succeeded despite Nitronode error")
	} else if got, want := err.Error(), "failed to submit app state: nitronode rejected stale app version"; got != want {
		t.Fatalf("SubmitUpdate() error = %q, want %q", got, want)
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

func mustEnvSigner(t *testing.T, privateKey string) appsigning.Signer {
	t.Helper()

	signer, err := appsigning.NewEnvSigner(privateKey)
	if err != nil {
		t.Fatalf("NewEnvSigner() error = %v", err)
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
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, _ bool) (*core.State, error) {
			return fundedHomeState(wallet, asset, "7"), nil
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

func signAppStateUpdateWithSessionKey(update app.AppStateUpdateV1, signer appsigning.Signer) (string, error) {
	payload, err := app.PackAppStateUpdateV1(update)
	if err != nil {
		return "", err
	}
	msgSigner, err := sign.NewEthereumMsgSignerFromRaw(signer.TxSigner())
	if err != nil {
		return "", err
	}
	sessionSigner, err := app.NewAppSessionKeySignerV1(msgSigner)
	if err != nil {
		return "", err
	}
	signature, err := sessionSigner.Sign(payload)
	if err != nil {
		return "", err
	}
	return signature.String(), nil
}
