package smoke_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/layer-3/nitrolite-go-example/internal/config"
	internalhttp "github.com/layer-3/nitrolite-go-example/internal/httpapi"
	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite-go-example/internal/store"
	"github.com/layer-3/nitrolite-go-example/internal/testsupport"
	"github.com/layer-3/nitrolite/pkg/core"
	"github.com/shopspring/decimal"
)

func TestScaffoldRoutes(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:               "8080",
		LogLevel:           "info",
		ClearnodeWSURL:     "wss://example.invalid",
		DemoPrivateKey:     "0x4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318",
		ConsoleAPIKey:      "12345678901234567890123456789012",
		BlockchainRPCURLs:  map[string]string{"11155111": "https://example.invalid"},
		HomeBlockchains:    map[string]uint64{"yellow": 11155111, "yusd": 11155111},
		SQLitePath:         filepath.Join(t.TempDir(), "smoke.db"),
		StoreName:          "Nitrolite App Session Store",
		StoreAppID:         "store",
		StoreAppPrivateKey: "",
	}

	appSigner, err := signing.NewStoreAppSigner(cfg.StoreAppPrivateKey, cfg.DemoPrivateKey)
	if err != nil {
		t.Fatalf("NewStoreAppSigner() error = %v", err)
	}

	client := &testsupport.FakeClient{
		GetBalancesFunc: func(context.Context, string) ([]core.BalanceEntry, error) {
			return []core.BalanceEntry{{Asset: "yusd", Balance: decimal.RequireFromString("10.5")}}, nil
		},
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
	t.Cleanup(func() {
		_ = appStore.Close()
	})

	handler, err := internalhttp.NewHandler(cfg, manager, appSigner, appSigner, appStore, slog.Default())
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	t.Run("healthz", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if payload["status"] != "ok" {
			t.Fatalf("status field = %v, want ok", payload["status"])
		}
	})

	t.Run("root", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
			t.Fatalf("root page missing SPA shell: %s", rec.Body.String())
		}
	})

	t.Run("reference removed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/reference", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("advanced removed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/advanced", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("bootstrap requires auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/store/bootstrap?asset=yusd", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
		}
	})
}
