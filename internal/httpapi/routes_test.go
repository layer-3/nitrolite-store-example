package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/layer-3/nitrolite-store-example/internal/config"
	"github.com/layer-3/nitrolite-store-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-store-example/internal/service"
	"github.com/layer-3/nitrolite-store-example/internal/signing"
	"github.com/layer-3/nitrolite-store-example/internal/store"
	"github.com/layer-3/nitrolite-store-example/internal/testsupport"
	"github.com/layer-3/nitrolite/pkg/core"
	"github.com/shopspring/decimal"
)

const httpAPITestPrivateKey = "0x4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"

func fundedHTTPState(wallet string, asset string, balance string) *core.State {
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

func TestStoreBootstrapRequiresWalletAddress(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{})
	req := httptest.NewRequest(http.MethodGet, "/api/store/bootstrap?asset=yusd", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var payload errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Error.Code != "invalid_request" {
		t.Fatalf("error code = %q, want invalid_request", payload.Error.Code)
	}
}

func TestStoreBootstrapWithWalletAddress(t *testing.T) {
	t.Parallel()

	walletAddress := "0x1111111111111111111111111111111111111111"
	handler := newTestHandler(t, &testsupport.FakeClient{
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, _ bool) (*core.State, error) {
			balances := map[string]string{"yusd": "7", "yellow": "3"}
			return fundedHTTPState(wallet, asset, balances[asset]), nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/store/bootstrap?asset=yusd&wallet_address="+walletAddress, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload struct {
		WalletAddress    string           `json:"wallet_address"`
		SelectedAsset    string           `json:"selected_asset"`
		SupportedAssets  []string         `json:"supported_assets"`
		AssetDecimals    map[string]uint8 `json:"asset_decimals"`
		AvailableBalance string           `json:"available_balance"`
		ChannelReadiness struct {
			Status                  string `json:"status"`
			HomeBlockchainID        uint64 `json:"home_blockchain_id"`
			BootstrapAmount         string `json:"bootstrap_amount"`
			RequiresChannelCreation bool   `json:"requires_channel_creation"`
		} `json:"channel_readiness"`
		Catalog []struct {
			ID string `json:"id"`
		} `json:"catalog"`
		Session struct {
			Status         string `json:"status"`
			UserAllocation string `json:"user_allocation"`
			AppAllocation  string `json:"app_allocation"`
		} `json:"session"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if payload.WalletAddress != walletAddress {
		t.Fatalf("wallet_address = %q, want %q", payload.WalletAddress, walletAddress)
	}
	if payload.SelectedAsset != "yusd" {
		t.Fatalf("selected_asset = %q, want yusd", payload.SelectedAsset)
	}
	if !reflect.DeepEqual(payload.SupportedAssets, []string{"yusd", "yellow"}) {
		t.Fatalf("supported_assets = %#v, want yusd/yellow", payload.SupportedAssets)
	}
	if payload.AssetDecimals["yusd"] != 6 {
		t.Fatalf("asset_decimals[yusd] = %d, want 6", payload.AssetDecimals["yusd"])
	}
	if payload.AssetDecimals["yellow"] != 18 {
		t.Fatalf("asset_decimals[yellow] = %d, want 18", payload.AssetDecimals["yellow"])
	}
	if payload.AvailableBalance != "7" {
		t.Fatalf("available_balance = %q, want 7", payload.AvailableBalance)
	}
	if payload.ChannelReadiness.Status != "ready" {
		t.Fatalf("channel_readiness.status = %q, want ready", payload.ChannelReadiness.Status)
	}
	if payload.ChannelReadiness.HomeBlockchainID != 11155111 {
		t.Fatalf("channel_readiness.home_blockchain_id = %d, want 11155111", payload.ChannelReadiness.HomeBlockchainID)
	}
	if payload.ChannelReadiness.BootstrapAmount != "10" {
		t.Fatalf("channel_readiness.bootstrap_amount = %q, want 10", payload.ChannelReadiness.BootstrapAmount)
	}
	if payload.ChannelReadiness.RequiresChannelCreation {
		t.Fatal("channel_readiness.requires_channel_creation = true, want false")
	}
	if payload.Session.Status != "missing" {
		t.Fatalf("session.status = %q, want missing", payload.Session.Status)
	}
	if payload.Session.UserAllocation != "0" || payload.Session.AppAllocation != "0" {
		t.Fatalf("unexpected session allocations = %#v", payload.Session)
	}
	if len(payload.Catalog) == 0 {
		t.Fatal("expected seeded catalog entries")
	}
}

func TestStoreBootstrapReportsReceivedFundsNeedChannelCreation(t *testing.T) {
	t.Parallel()

	walletAddress := "0x1111111111111111111111111111111111111111"
	handler := newTestHandler(t, &testsupport.FakeClient{
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, signed bool) (*core.State, error) {
			if signed {
				return nil, errors.New("signed state not found")
			}
			state := fundedHTTPState(wallet, asset, "5")
			state.HomeChannelID = nil
			return state, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/store/bootstrap?asset=yusd&wallet_address="+walletAddress, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload struct {
		AvailableBalance string `json:"available_balance"`
		ChannelReadiness struct {
			Status                  string `json:"status"`
			PendingBalance          string `json:"pending_balance"`
			RequiresChannelCreation bool   `json:"requires_channel_creation"`
		} `json:"channel_readiness"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.ChannelReadiness.Status != "ack_required" {
		t.Fatalf("channel_readiness.status = %q, want ack_required", payload.ChannelReadiness.Status)
	}
	if !payload.ChannelReadiness.RequiresChannelCreation {
		t.Fatal("channel_readiness.requires_channel_creation = false, want true")
	}
	if payload.AvailableBalance != "0" {
		t.Fatalf("available_balance = %q, want 0", payload.AvailableBalance)
	}
	if payload.ChannelReadiness.PendingBalance != "5" {
		t.Fatalf("channel_readiness.pending_balance = %q, want 5", payload.ChannelReadiness.PendingBalance)
	}
}

func TestStoreBootstrapWithYellowAsset(t *testing.T) {
	t.Parallel()

	walletAddress := "0x1111111111111111111111111111111111111111"
	handler := newTestHandler(t, &testsupport.FakeClient{
		GetLatestStateFunc: func(_ context.Context, wallet string, asset string, _ bool) (*core.State, error) {
			balances := map[string]string{"yusd": "7", "yellow": "3"}
			return fundedHTTPState(wallet, asset, balances[asset]), nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/store/bootstrap?asset=yellow&wallet_address="+walletAddress, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload struct {
		SelectedAsset    string `json:"selected_asset"`
		AvailableBalance string `json:"available_balance"`
		Catalog          []struct {
			ID     string            `json:"id"`
			Prices map[string]string `json:"prices"`
		} `json:"catalog"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.SelectedAsset != "yellow" {
		t.Fatalf("selected_asset = %q, want yellow", payload.SelectedAsset)
	}
	if payload.AvailableBalance != "3" {
		t.Fatalf("available_balance = %q, want 3", payload.AvailableBalance)
	}
	if len(payload.Catalog) == 0 || payload.Catalog[0].Prices["yellow"] != "1.35" {
		t.Fatalf("unexpected yellow catalog = %#v", payload.Catalog)
	}
}

func TestStoreContentSignedPostDisabled(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{})

	req := httptest.NewRequest(http.MethodPost, "/api/store/content/1/open", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
}

func TestStoreBootstrapReportsUnavailableWhenDisconnected(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	appSigner, err := signing.NewStoreAppSigner("", httpAPITestPrivateKey)
	if err != nil {
		t.Fatalf("NewStoreAppSigner() error = %v", err)
	}

	manager := nitrolite.NewManagerWithClient(&testsupport.FakeClient{}, nitrolite.Health{
		Connected:     false,
		Ready:         false,
		SignerAddress: appSigner.Address(),
	}, slog.Default())
	appStore, err := store.New(cfg.SQLitePath)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	t.Cleanup(func() { _ = appStore.Close() })

	handler, err := NewHandler(cfg, manager, appSigner, appSigner, appStore, slog.Default())
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/store/bootstrap?asset=yusd&wallet_address=0x1111111111111111111111111111111111111111", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload struct {
		AvailableBalance string `json:"available_balance"`
		ChannelReadiness struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"channel_readiness"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.AvailableBalance != "0" {
		t.Fatalf("available_balance = %q, want 0", payload.AvailableBalance)
	}
	if payload.ChannelReadiness.Status != "unavailable" {
		t.Fatalf("channel_readiness.status = %q, want unavailable", payload.ChannelReadiness.Status)
	}
}

func TestWriteServiceErrorUnavailableUsesNitronodeCode(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	writeServiceError(rec, service.ErrUnavailable)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}

	var payload errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Error.Code != "nitronode_unavailable" {
		t.Fatalf("error code = %q, want nitronode_unavailable", payload.Error.Code)
	}
	if payload.Error.Message != "nitronode not reachable" {
		t.Fatalf("error message = %q, want nitronode not reachable", payload.Error.Message)
	}
}

func TestWriteServiceErrorUpstreamUsesNitronodeCode(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	writeServiceError(rec, upstreamMappingError{})

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}

	var payload errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Error.Code != "nitronode_operation_failed" {
		t.Fatalf("error code = %q, want nitronode_operation_failed", payload.Error.Code)
	}
}

type upstreamMappingError struct{}

func (upstreamMappingError) Error() string {
	return "upstream mapping test"
}

func (upstreamMappingError) As(target any) bool {
	upstreamErr, ok := target.(*service.UpstreamError)
	if !ok {
		return false
	}
	*upstreamErr = service.UpstreamError{}
	return true
}

func newTestHandler(t *testing.T, client *testsupport.FakeClient) http.Handler {
	t.Helper()
	return newTestHarness(t, client).handler
}

type testHarness struct {
	handler   http.Handler
	store     *store.Store
	appSigner signing.Signer
}

func newTestHarness(t *testing.T, client *testsupport.FakeClient) testHarness {
	t.Helper()

	cfg := testConfig(t)
	appSigner, err := signing.NewStoreAppSigner("", httpAPITestPrivateKey)
	if err != nil {
		t.Fatalf("NewStoreAppSigner() error = %v", err)
	}

	manager := nitrolite.NewManagerWithClient(client, nitrolite.Health{
		Connected:     true,
		Ready:         true,
		SignerAddress: appSigner.Address(),
	}, slog.Default())
	appStore, err := store.New(cfg.SQLitePath)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	t.Cleanup(func() { _ = appStore.Close() })

	handler, err := NewHandler(cfg, manager, appSigner, appSigner, appStore, slog.Default())
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	return testHarness{handler: handler, store: appStore, appSigner: appSigner}
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Port:               "8080",
		LogLevel:           "info",
		ClearnodeWSURL:     "wss://example.invalid",
		DemoPrivateKey:     httpAPITestPrivateKey,
		BlockchainRPCURLs:  map[string]string{"11155111": "https://example.invalid"},
		HomeBlockchains:    map[string]uint64{"yellow": 11155111, "yusd": 11155111},
		SQLitePath:         filepath.Join(t.TempDir(), "test.db"),
		StoreName:          "Nitrolite App Session Store",
		StoreAppID:         "store",
		StoreAppPrivateKey: "",
		StoreChannelBootstrapAmounts: map[string]string{
			"yellow": "10",
			"yusd":   "10",
		},
	}
}
