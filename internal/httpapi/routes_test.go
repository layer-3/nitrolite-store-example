package httpapi

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
	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite-go-example/internal/store"
	"github.com/layer-3/nitrolite-go-example/internal/testsupport"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
	"github.com/layer-3/nitrolite/pkg/sign"
	sdk "github.com/layer-3/nitrolite/sdk/go"
	"github.com/shopspring/decimal"
)

const httpAPITestPrivateKey = "0x4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"

func TestStoreBootstrapRequiresWalletConnection(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{})
	req := httptest.NewRequest(http.MethodGet, "/api/store/bootstrap?asset=yusd", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	var payload errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Error.Code != "unauthorized" {
		t.Fatalf("error code = %q, want unauthorized", payload.Error.Code)
	}
}

func TestStoreConnectVerifyAndBootstrap(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{
		GetBalancesFunc: func(context.Context, string) ([]core.BalanceEntry, error) {
			return []core.BalanceEntry{
				{Asset: "yusd", Balance: decimal.RequireFromString("7")},
				{Asset: "yellow", Balance: decimal.RequireFromString("3")},
			}, nil
		},
		GetAppSessionsFunc: func(context.Context, *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
			return nil, core.PaginationMetadata{}, nil
		},
	})
	userSigner := mustUserSigner(t)
	authCookie := connectWallet(t, handler, userSigner)

	req := httptest.NewRequest(http.MethodGet, "/api/store/bootstrap?asset=yusd", nil)
	req.AddCookie(authCookie)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload struct {
		StoreName        string `json:"store_name"`
		AppID            string `json:"app_id"`
		AppSigner        string `json:"app_signer"`
		WalletAddress    string `json:"wallet_address"`
		SelectedAsset    string `json:"selected_asset"`
		DefaultAsset     string `json:"default_asset"`
		SupportedAssets  []string `json:"supported_assets"`
		AvailableBalance string `json:"available_balance"`
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

	if payload.WalletAddress != userSigner.Address() {
		t.Fatalf("wallet_address = %q, want %q", payload.WalletAddress, userSigner.Address())
	}
	if payload.SelectedAsset != "yusd" {
		t.Fatalf("selected_asset = %q, want yusd", payload.SelectedAsset)
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
	if len(payload.SupportedAssets) == 0 {
		t.Fatal("expected supported_assets")
	}
}

func TestStoreConnectVerifyRejectsWrongSignature(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{})
	userSigner := mustUserSigner(t)
	challenge := requestChallenge(t, handler, userSigner.Address())
	wrongSigner, err := signing.NewStoreAppSigner("", httpAPITestPrivateKey)
	if err != nil {
		t.Fatalf("NewStoreAppSigner() error = %v", err)
	}

	verifyReq := httptest.NewRequest(http.MethodPost, "/api/store/connect/verify", strings.NewReader(mustMarshalJSON(t, map[string]string{
		"challenge_id":   challenge.ChallengeID,
		"wallet_address": userSigner.Address(),
		"signature":      signChallengeMessage(t, wrongSigner, challenge.Message),
	})))
	verifyRec := httptest.NewRecorder()
	handler.ServeHTTP(verifyRec, verifyReq)

	if verifyRec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d body=%s", verifyRec.Code, http.StatusUnauthorized, verifyRec.Body.String())
	}
}

func TestStoreContentRequiresOwnership(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{
		GetBalancesFunc: func(context.Context, string) ([]core.BalanceEntry, error) {
			return []core.BalanceEntry{{Asset: "yusd", Balance: decimal.RequireFromString("7")}}, nil
		},
	})
	authCookie := connectWallet(t, handler, mustUserSigner(t))

	req := httptest.NewRequest(http.MethodGet, "/api/store/content/article-micropayments?asset=yusd", nil)
	req.AddCookie(authCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestStoreBootstrapReturnsServiceUnavailableWhenDisconnected(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:               "8080",
		LogLevel:           "info",
		ClearnodeWSURL:     "wss://example.invalid",
		DemoPrivateKey:     httpAPITestPrivateKey,
		ConsoleAPIKey:      "12345678901234567890123456789012",
		BlockchainRPCURLs:  map[string]string{"11155111": "https://example.invalid"},
		HomeBlockchains:    map[string]uint64{"yellow": 11155111, "yusd": 11155111},
		SQLitePath:         filepath.Join(t.TempDir(), "test.db"),
		StoreName:          "Nitrolite App Session Store",
		StoreAppID:         "store",
		StoreAppPrivateKey: "",
	}
	userSigner := mustUserSigner(t)
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

	handler, err := NewHandler(cfg, manager, userSigner, appSigner, appStore, slog.Default())
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	authCookie := connectWallet(t, handler, userSigner)

	req := httptest.NewRequest(http.MethodGet, "/api/store/bootstrap?asset=yusd", nil)
	req.AddCookie(authCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

type testHarness struct {
	handler    http.Handler
	store      *store.Store
	userSigner signing.Signer
	appSigner  signing.Signer
}

func newTestHarness(t *testing.T, client *testsupport.FakeClient) testHarness {
	t.Helper()

	cfg := &config.Config{
		Port:               "8080",
		LogLevel:           "info",
		ClearnodeWSURL:     "wss://example.invalid",
		DemoPrivateKey:     httpAPITestPrivateKey,
		ConsoleAPIKey:      "12345678901234567890123456789012",
		BlockchainRPCURLs:  map[string]string{"11155111": "https://example.invalid"},
		HomeBlockchains:    map[string]uint64{"yellow": 11155111, "yusd": 11155111},
		SQLitePath:         filepath.Join(t.TempDir(), "test.db"),
		StoreName:          "Nitrolite App Session Store",
		StoreAppID:         "store",
		StoreAppPrivateKey: "",
	}
	userSigner := mustUserSigner(t)
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

	handler, err := NewHandler(cfg, manager, userSigner, appSigner, appStore, slog.Default())
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	return testHarness{handler: handler, store: appStore, userSigner: userSigner, appSigner: appSigner}
}

func newTestHandler(t *testing.T, client *testsupport.FakeClient) http.Handler {
	t.Helper()
	return newTestHarness(t, client).handler
}

type storeChallengeResponse struct {
	ChallengeID string `json:"challenge_id"`
	Message     string `json:"message"`
}

func requestChallenge(t *testing.T, handler http.Handler, walletAddress string) storeChallengeResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/store/connect/challenge", strings.NewReader(mustMarshalJSON(t, map[string]string{
		"wallet_address": walletAddress,
	})))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("challenge status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload storeChallengeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.ChallengeID == "" || payload.Message == "" {
		t.Fatalf("unexpected challenge payload = %#v", payload)
	}
	return payload
}

func connectWallet(t *testing.T, handler http.Handler, signer signing.Signer) *http.Cookie {
	t.Helper()

	challenge := requestChallenge(t, handler, signer.Address())
	req := httptest.NewRequest(http.MethodPost, "/api/store/connect/verify", strings.NewReader(mustMarshalJSON(t, map[string]string{
		"challenge_id":   challenge.ChallengeID,
		"wallet_address": signer.Address(),
		"signature":      signChallengeMessage(t, signer, challenge.Message),
	})))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("verify status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cookie := storeAuthCookie(t, rec.Result().Cookies())
	if cookie.Value == "" {
		t.Fatal("expected auth cookie value")
	}
	return cookie
}

func mustUserSigner(t *testing.T) signing.Signer {
	t.Helper()

	signer, err := signing.NewEnvSigner(httpAPITestPrivateKey)
	if err != nil {
		t.Fatalf("NewEnvSigner() error = %v", err)
	}
	return signer
}

func signChallengeMessage(t *testing.T, signer signing.Signer, message string) string {
	t.Helper()

	msgSigner, err := sign.NewEthereumMsgSignerFromRaw(signer.TxSigner())
	if err != nil {
		t.Fatalf("NewEthereumMsgSignerFromRaw() error = %v", err)
	}
	signature, err := msgSigner.Sign([]byte(message))
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	return signature.String()
}

func mustMarshalJSON(t *testing.T, payload any) string {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(body)
}

func storeAuthCookie(t *testing.T, cookies []*http.Cookie) *http.Cookie {
	t.Helper()

	for _, cookie := range cookies {
		if cookie.Name == storeAuthSessionCookieName {
			return cookie
		}
	}
	t.Fatal("expected store auth cookie")
	return nil
}
