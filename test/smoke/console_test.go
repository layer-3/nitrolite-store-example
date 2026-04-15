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
	"time"

	"github.com/layer-3/nitrolite-go-example/internal/config"
	internalhttp "github.com/layer-3/nitrolite-go-example/internal/httpapi"
	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite-go-example/internal/store"
	"github.com/layer-3/nitrolite-go-example/internal/testsupport"
	"github.com/layer-3/nitrolite/pkg/core"
	sdk "github.com/layer-3/nitrolite/sdk/go"
	"github.com/shopspring/decimal"
)

func TestScaffoldRoutes(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:              "8080",
		LogLevel:          "info",
		ClearnodeWSURL:    "wss://example.invalid",
		DemoPrivateKey:    "0x4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318",
		ConsoleAPIKey:     "12345678901234567890123456789012",
		BlockchainRPCURLs: map[string]string{"80002": "https://example.invalid"},
		HomeBlockchains:   map[string]uint64{"usdc": 80002},
		SQLitePath:        filepath.Join(t.TempDir(), "smoke.db"),
		MerchantName:      "Nitrolite Sandbox Merchant",
		MerchantAppID:     "default",
	}

	client := &testsupport.FakeClient{
		GetUserAddressFunc: func() string { return "0xabc" },
		GetConfigFunc: func(context.Context) (*core.NodeConfig, error) {
			return &core.NodeConfig{
				NodeAddress:            "0xnode",
				NodeVersion:            "1.2.3",
				SupportedSigValidators: []core.ChannelSignerType{core.ChannelSignerType_Default},
				Blockchains: []core.Blockchain{
					{
						ID:                     80002,
						Name:                   "Polygon Amoy",
						ChannelHubAddress:      "0xhub",
						LockingContractAddress: "0xlock",
						BlockStep:              1,
					},
				},
			}, nil
		},
		GetBlockchainsFunc: func(context.Context) ([]core.Blockchain, error) {
			return []core.Blockchain{
				{
					ID:                     80002,
					Name:                   "Polygon Amoy",
					ChannelHubAddress:      "0xhub",
					LockingContractAddress: "0xlock",
					BlockStep:              1,
				},
			}, nil
		},
		GetAssetsFunc: func(context.Context, *uint64) ([]core.Asset, error) {
			return []core.Asset{
				{
					Name:                  "USD Coin",
					Symbol:                "usdc",
					Decimals:              6,
					SuggestedBlockchainID: 80002,
					Tokens: []core.Token{
						{
							Name:         "USD Coin",
							Symbol:       "USDC",
							Address:      "0xtoken",
							BlockchainID: 80002,
							Decimals:     6,
						},
					},
				},
			}, nil
		},
		GetBalancesFunc: func(context.Context, string) ([]core.BalanceEntry, error) {
			return []core.BalanceEntry{{Asset: "usdc", Balance: decimal.RequireFromString("10.5")}}, nil
		},
		GetTransactionsFunc: func(context.Context, string, *sdk.GetTransactionsOptions) ([]core.Transaction, core.PaginationMetadata, error) {
			return []core.Transaction{
					{
						ID:          "tx-1",
						Asset:       "usdc",
						TxType:      core.TransactionTypeTransfer,
						FromAccount: "0xabc",
						ToAccount:   "0xdef",
						Amount:      decimal.RequireFromString("1.25"),
						CreatedAt:   time.Unix(1700000000, 0).UTC(),
					},
				}, core.PaginationMetadata{
					Page:       1,
					PerPage:    20,
					TotalCount: 1,
					PageCount:  1,
				}, nil
		},
		GetHomeChannelFunc: func(context.Context, string, string) (*core.Channel, error) {
			return &core.Channel{
				ChannelID:             "0xchannel",
				UserWallet:            "0xabc",
				Asset:                 "usdc",
				Type:                  core.ChannelTypeHome,
				BlockchainID:          80002,
				TokenAddress:          "0xtoken",
				ChallengeDuration:     3600,
				Nonce:                 7,
				ApprovedSigValidators: "0x01",
				Status:                core.ChannelStatusOpen,
				StateVersion:          5,
			}, nil
		},
		GetLatestStateFunc: func(context.Context, string, string, bool) (*core.State, error) {
			homeChannelID := "0xchannel"
			userSig := "0xusersig"
			nodeSig := "0xnodesig"
			return &core.State{
				ID:            "state-1",
				Asset:         "usdc",
				UserWallet:    "0xabc",
				Epoch:         1,
				Version:       5,
				HomeChannelID: &homeChannelID,
				Transition: core.Transition{
					Type:      core.TransitionTypeHomeDeposit,
					TxID:      "0xtx",
					AccountID: "0xaccount",
					Amount:    decimal.RequireFromString("5"),
				},
				HomeLedger: core.Ledger{
					TokenAddress: "0xtoken",
					BlockchainID: 80002,
					UserBalance:  decimal.RequireFromString("10"),
					UserNetFlow:  decimal.RequireFromString("10"),
					NodeBalance:  decimal.Zero,
					NodeNetFlow:  decimal.Zero,
				},
				UserSig: &userSig,
				NodeSig: &nodeSig,
			}, nil
		},
	}

	manager := nitrolite.NewManagerWithClient(client, nitrolite.Health{
		Connected:     true,
		Ready:         true,
		SignerAddress: "0xabc",
	}, slog.Default())

	signer, err := signing.NewEnvSigner(cfg.DemoPrivateKey)
	if err != nil {
		t.Fatalf("NewEnvSigner() error = %v", err)
	}
	appStore, err := store.New(cfg.SQLitePath)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = appStore.Close()
	})

	handler, err := internalhttp.NewHandler(cfg, manager, signer, appStore, slog.Default())
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
		if !strings.Contains(rec.Body.String(), "Merchant settlement dashboard") {
			t.Fatalf("root page missing merchant dashboard copy: %s", rec.Body.String())
		}
	})

	t.Run("reference", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/reference", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), "Embedded API reference") {
			t.Fatalf("reference page missing expected copy: %s", rec.Body.String())
		}
	})

	t.Run("advanced", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/advanced", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), "Advanced operator console") {
			t.Fatalf("advanced page missing expected copy: %s", rec.Body.String())
		}
	})

	t.Run("pay page", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/pay/demo-slug", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), "Sandbox payment request") {
			t.Fatalf("pay page missing expected copy: %s", rec.Body.String())
		}
	})

	t.Run("openapi", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if payload["openapi"] != "3.1.0" {
			t.Fatalf("openapi version = %v, want 3.1.0", payload["openapi"])
		}
	})

	t.Run("node config", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/node/config", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if payload["nodeAddress"] != "0xnode" {
			t.Fatalf("nodeAddress = %v, want 0xnode", payload["nodeAddress"])
		}
	})

	t.Run("balances", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/balances", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var payload struct {
			Balances []struct {
				Asset   string `json:"asset"`
				Balance string `json:"balance"`
			} `json:"balances"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if len(payload.Balances) != 1 || payload.Balances[0].Balance != "10.5" {
			t.Fatalf("balances = %#v, want one usdc balance", payload.Balances)
		}
	})

	t.Run("channel missing asset", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/channel", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("channel state", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/channel/state?asset=usdc&only_signed=true", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		state, ok := payload["state"].(map[string]any)
		if !ok {
			t.Fatalf("state payload missing: %#v", payload)
		}
		if state["version"] != float64(5) {
			t.Fatalf("version = %v, want 5", state["version"])
		}
	})
}
