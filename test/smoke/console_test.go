package smoke_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/layer-3/nitrolite-go-example/internal/config"
	internalhttp "github.com/layer-3/nitrolite-go-example/internal/httpapi"
	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
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
		DemoPrivateKey:    "0xdeadbeef",
		ConsoleAPIKey:     "12345678901234567890123456789012",
		BlockchainRPCURLs: map[string]string{"80002": "https://example.invalid"},
		HomeBlockchains:   map[string]uint64{"usdc": 80002},
	}

	manager := nitrolite.NewManagerWithClient(&fakeClient{}, nitrolite.Health{
		Connected:     true,
		Ready:         true,
		SignerAddress: "0xabc",
	}, slog.Default())
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

type fakeClient struct{}

func (f *fakeClient) Close() error {
	return nil
}

func (f *fakeClient) GetUserAddress() string {
	return "0xabc"
}

func (f *fakeClient) Ping(context.Context) error {
	return nil
}

func (f *fakeClient) SetHomeBlockchain(string, uint64) error {
	return nil
}

func (f *fakeClient) WaitCh() <-chan struct{} {
	ch := make(chan struct{})
	return ch
}

func (f *fakeClient) GetConfig(context.Context) (*core.NodeConfig, error) {
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
}

func (f *fakeClient) GetBlockchains(context.Context) ([]core.Blockchain, error) {
	cfg, _ := f.GetConfig(context.Background())
	return cfg.Blockchains, nil
}

func (f *fakeClient) GetAssets(context.Context, *uint64) ([]core.Asset, error) {
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
}

func (f *fakeClient) GetBalances(context.Context, string) ([]core.BalanceEntry, error) {
	return []core.BalanceEntry{
		{
			Asset:   "usdc",
			Balance: decimal.RequireFromString("10.5"),
		},
	}, nil
}

func (f *fakeClient) GetTransactions(context.Context, string, *sdk.GetTransactionsOptions) ([]core.Transaction, core.PaginationMetadata, error) {
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
}

func (f *fakeClient) GetHomeChannel(context.Context, string, string) (*core.Channel, error) {
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
}

func (f *fakeClient) GetLatestState(context.Context, string, string, bool) (*core.State, error) {
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
}
