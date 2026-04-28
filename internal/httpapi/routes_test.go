package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/layer-3/nitrolite-store-example/internal/config"
	"github.com/layer-3/nitrolite-store-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-store-example/internal/signing"
	"github.com/layer-3/nitrolite-store-example/internal/store"
	"github.com/layer-3/nitrolite-store-example/internal/testsupport"
	"github.com/layer-3/nitrolite/pkg/core"
	"github.com/shopspring/decimal"
)

const httpAPITestPrivateKey = "0x4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"

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
		GetBalancesFunc: func(context.Context, string) ([]core.BalanceEntry, error) {
			return []core.BalanceEntry{
				{Asset: "yusd", Balance: decimal.RequireFromString("7")},
				{Asset: "yellow", Balance: decimal.RequireFromString("3")},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/store/bootstrap?asset=yusd&wallet_address="+walletAddress, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload struct {
		WalletAddress    string   `json:"wallet_address"`
		SelectedAsset    string   `json:"selected_asset"`
		SupportedAssets  []string `json:"supported_assets"`
		AvailableBalance string   `json:"available_balance"`
		Catalog          []struct {
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
	if payload.AvailableBalance != "7" {
		t.Fatalf("available_balance = %q, want 7", payload.AvailableBalance)
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

func TestStoreBootstrapWithYellowAsset(t *testing.T) {
	t.Parallel()

	walletAddress := "0x1111111111111111111111111111111111111111"
	handler := newTestHandler(t, &testsupport.FakeClient{
		GetBalancesFunc: func(context.Context, string) ([]core.BalanceEntry, error) {
			return []core.BalanceEntry{
				{Asset: "yusd", Balance: decimal.RequireFromString("7")},
				{Asset: "yellow", Balance: decimal.RequireFromString("3")},
			}, nil
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

func TestStoreBootstrapReturnsServiceUnavailableWhenDisconnected(t *testing.T) {
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

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
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
	}
}
