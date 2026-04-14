package smoke_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/layer-3/nitrolite-go-example/internal/config"
	internalhttp "github.com/layer-3/nitrolite-go-example/internal/httpapi"
	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
)

func TestScaffoldRoutes(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:              "8080",
		LogLevel:          "info",
		ClearnodeWSURL:    "wss://example.invalid",
		DemoPrivateKey:    "0xdeadbeef",
		ConsoleAPIKey:     "12345678901234567890123456789012",
		BlockchainRPCURLs: map[string]string{"80002": "https://example.invalid"},
		HomeBlockchains:   map[string]uint64{"usdc": 80002},
	}

	manager := nitrolite.NewManager("0xabc")
	handler, err := internalhttp.NewHandler(cfg, manager, slog.Default())
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
		if contentType := rec.Header().Get("Content-Type"); contentType == "" {
			t.Fatal("missing Content-Type")
		}
	})
}
