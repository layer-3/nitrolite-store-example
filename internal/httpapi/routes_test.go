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
	sdk "github.com/layer-3/nitrolite/sdk/go"
	"github.com/shopspring/decimal"
)

const httpAPITestPrivateKey = "0x4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"

func TestProtectedMutationRequiresAPIKey(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/register", strings.NewReader(`{"app_id":"demo-app","metadata":"","creation_approval_not_required":true}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	errPayload := payload["error"].(map[string]any)
	if errPayload["code"] != "unauthorized" {
		t.Fatalf("error code = %v", errPayload["code"])
	}
}

func TestAuthUnlockSetsCookieAndAllowsMutation(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{})

	unlockReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/unlock", strings.NewReader(`{"api_key":"12345678901234567890123456789012"}`))
	unlockRec := httptest.NewRecorder()
	handler.ServeHTTP(unlockRec, unlockReq)

	if unlockRec.Code != http.StatusOK {
		t.Fatalf("unlock status = %d, want %d body=%s", unlockRec.Code, http.StatusOK, unlockRec.Body.String())
	}

	cookie := unlockCookie(t, unlockRec.Result().Cookies())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/register", strings.NewReader(`{"app_id":"demo-app","metadata":"","creation_approval_not_required":true}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestStoreConfigCreatesBrowserCookie(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/config", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cookie := browserCookie(t, rec.Result().Cookies())
	if cookie.Value == "" {
		t.Fatal("expected browser cookie value")
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	storePayload := payload["store"].(map[string]any)
	if storePayload["store_name"] == "" {
		t.Fatalf("unexpected store payload = %#v", storePayload)
	}
}

func TestStoreSessionDepositPurchaseAndContentFlow(t *testing.T) {
	t.Parallel()

	harness := newStoreHarness(t)

	browser := newBrowserSession(t, harness.handler)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/store/session/create", strings.NewReader(`{"asset":"yusd"}`))
	createReq.AddCookie(browser)
	createRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d body=%s", createRec.Code, http.StatusOK, createRec.Body.String())
	}

	var createPayload struct {
		Session struct {
			AppSessionID string `json:"app_session_id"`
			Status       string `json:"status"`
			Version      uint64 `json:"version"`
		} `json:"session"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if createPayload.Session.AppSessionID == "" || createPayload.Session.Status != "open" || createPayload.Session.Version != 1 {
		t.Fatalf("unexpected create payload = %#v", createPayload)
	}

	depositReq := httptest.NewRequest(http.MethodPost, "/api/v1/app-session/submit-state", strings.NewReader(`{"session_id":"0xstore","asset":"yusd","session_data":"{\"action\":\"deposit\",\"amount\":\"1.00\"}"}`))
	depositReq.AddCookie(browser)
	depositRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(depositRec, depositReq)

	if depositRec.Code != http.StatusOK {
		t.Fatalf("deposit status = %d, want %d body=%s", depositRec.Code, http.StatusOK, depositRec.Body.String())
	}

	var depositPayload struct {
		Session struct {
			Version        uint64 `json:"version"`
			UserAllocation string `json:"user_allocation"`
			SessionData    string `json:"session_data"`
		} `json:"session"`
	}
	if err := json.Unmarshal(depositRec.Body.Bytes(), &depositPayload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if depositPayload.Session.Version != 2 || depositPayload.Session.UserAllocation != "1" {
		t.Fatalf("unexpected deposit payload = %#v", depositPayload)
	}
	if !strings.Contains(depositPayload.Session.SessionData, `"action":"deposit"`) {
		t.Fatalf("deposit session_data = %q", depositPayload.Session.SessionData)
	}

	purchaseReq := httptest.NewRequest(http.MethodPost, "/api/v1/app-session/submit-state", strings.NewReader(`{"session_id":"0xstore","asset":"yusd","session_data":"{\"action\":\"purchase\",\"item_id\":\"article-micropayments\",\"price\":\"0.50\"}"}`))
	purchaseReq.AddCookie(browser)
	purchaseRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(purchaseRec, purchaseReq)

	if purchaseRec.Code != http.StatusOK {
		t.Fatalf("purchase status = %d, want %d body=%s", purchaseRec.Code, http.StatusOK, purchaseRec.Body.String())
	}

	var purchasePayload struct {
		Session struct {
			Version        uint64 `json:"version"`
			UserAllocation string `json:"user_allocation"`
			AppAllocation  string `json:"app_allocation"`
		} `json:"session"`
	}
	if err := json.Unmarshal(purchaseRec.Body.Bytes(), &purchasePayload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if purchasePayload.Session.Version != 3 || purchasePayload.Session.UserAllocation != "0.5" || purchasePayload.Session.AppAllocation != "0.5" {
		t.Fatalf("unexpected purchase payload = %#v", purchasePayload)
	}

	contentReq := httptest.NewRequest(http.MethodGet, "/api/v1/content/article-micropayments?asset=yusd", nil)
	contentReq.AddCookie(browser)
	contentRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(contentRec, contentReq)

	if contentRec.Code != http.StatusOK {
		t.Fatalf("content status = %d, want %d body=%s", contentRec.Code, http.StatusOK, contentRec.Body.String())
	}

	var contentPayload struct {
		Item struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		} `json:"item"`
	}
	if err := json.Unmarshal(contentRec.Body.Bytes(), &contentPayload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if contentPayload.Item.ID != "article-micropayments" || contentPayload.Item.Content == "" {
		t.Fatalf("unexpected content payload = %#v", contentPayload)
	}
}

func TestAppWithdrawRequiresDeveloperAccess(t *testing.T) {
	t.Parallel()

	harness := newStoreHarness(t)
	browser := newBrowserSession(t, harness.handler)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/store/session/create", strings.NewReader(`{"asset":"yusd"}`))
	createReq.AddCookie(browser)
	createRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d body=%s", createRec.Code, http.StatusOK, createRec.Body.String())
	}

	depositReq := httptest.NewRequest(http.MethodPost, "/api/v1/app-session/submit-state", strings.NewReader(`{"session_id":"0xstore","asset":"yusd","session_data":"{\"action\":\"deposit\",\"amount\":\"1.00\"}"}`))
	depositReq.AddCookie(browser)
	depositRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(depositRec, depositReq)
	if depositRec.Code != http.StatusOK {
		t.Fatalf("deposit status = %d, want %d body=%s", depositRec.Code, http.StatusOK, depositRec.Body.String())
	}

	purchaseReq := httptest.NewRequest(http.MethodPost, "/api/v1/app-session/submit-state", strings.NewReader(`{"session_id":"0xstore","asset":"yusd","session_data":"{\"action\":\"purchase\",\"item_id\":\"article-micropayments\",\"price\":\"0.50\"}"}`))
	purchaseReq.AddCookie(browser)
	purchaseRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(purchaseRec, purchaseReq)
	if purchaseRec.Code != http.StatusOK {
		t.Fatalf("purchase status = %d, want %d body=%s", purchaseRec.Code, http.StatusOK, purchaseRec.Body.String())
	}

	appWithdrawReq := httptest.NewRequest(http.MethodPost, "/api/v1/app-session/submit-state", strings.NewReader(`{"session_id":"0xstore","asset":"yusd","session_data":"{\"action\":\"app_withdraw\",\"amount\":\"0.50\"}"}`))
	appWithdrawReq.AddCookie(browser)
	appWithdrawRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(appWithdrawRec, appWithdrawReq)

	if appWithdrawRec.Code != http.StatusConflict {
		t.Fatalf("app withdraw status = %d, want %d body=%s", appWithdrawRec.Code, http.StatusConflict, appWithdrawRec.Body.String())
	}

	writeCookie := unlockSession(t, harness.handler)
	appWithdrawReq = httptest.NewRequest(http.MethodPost, "/api/v1/app-session/submit-state", strings.NewReader(`{"session_id":"0xstore","asset":"yusd","session_data":"{\"action\":\"app_withdraw\",\"amount\":\"0.50\"}"}`))
	appWithdrawReq.AddCookie(browser)
	appWithdrawReq.AddCookie(writeCookie)
	appWithdrawRec = httptest.NewRecorder()
	harness.handler.ServeHTTP(appWithdrawRec, appWithdrawReq)

	if appWithdrawRec.Code != http.StatusOK {
		t.Fatalf("authorized app withdraw status = %d, want %d body=%s", appWithdrawRec.Code, http.StatusOK, appWithdrawRec.Body.String())
	}
}

func TestOpenAPIJSONStorePaths(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{})
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	paths := payload["paths"].(map[string]any)
	if _, ok := paths["/api/v1/store/config"]; !ok {
		t.Fatal("missing /api/v1/store/config path")
	}
	if _, ok := paths["/api/v1/app-session/submit-state"]; !ok {
		t.Fatal("missing /api/v1/app-session/submit-state path")
	}
	if _, ok := paths["/api/v1/payment-requests"]; ok {
		t.Fatal("unexpected merchant payment request path")
	}
}

func TestAppsEndpointReturnsServiceUnavailableWhenDisconnected(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:               "8080",
		LogLevel:           "info",
		ClearnodeWSURL:     "wss://example.invalid",
		DemoPrivateKey:     httpAPITestPrivateKey,
		ConsoleAPIKey:      "12345678901234567890123456789012",
		BlockchainRPCURLs:  map[string]string{"11155111": "https://example.invalid"},
		HomeBlockchains:    map[string]uint64{"yusd": 11155111},
		SQLitePath:         filepath.Join(t.TempDir(), "test.db"),
		StoreName:          "Nitrolite App Session Store",
		StoreAppID:         "store",
		StoreAppPrivateKey: "",
	}
	userSigner, err := signing.NewEnvSigner(httpAPITestPrivateKey)
	if err != nil {
		t.Fatalf("NewEnvSigner() error = %v", err)
	}
	appSigner, err := signing.NewStoreAppSigner("", httpAPITestPrivateKey)
	if err != nil {
		t.Fatalf("NewStoreAppSigner() error = %v", err)
	}

	client := &testsupport.FakeClient{
		GetUserAddressFunc: func() string { return userSigner.Address() },
	}
	manager := nitrolite.NewManagerWithClient(client, nitrolite.Health{
		Connected:     false,
		Ready:         false,
		SignerAddress: userSigner.Address(),
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

type testHarness struct {
	handler    http.Handler
	store      *store.Store
	userSigner signing.Signer
	appSigner  signing.Signer
}

func newStoreHarness(t *testing.T) testHarness {
	t.Helper()

	userSigner, err := signing.NewEnvSigner(httpAPITestPrivateKey)
	if err != nil {
		t.Fatalf("NewEnvSigner() error = %v", err)
	}
	appSigner, err := signing.NewStoreAppSigner("", httpAPITestPrivateKey)
	if err != nil {
		t.Fatalf("NewStoreAppSigner() error = %v", err)
	}

	currentSession := app.AppSessionInfoV1{
		AppSessionID: "0xstore",
		AppDefinition: app.AppDefinitionV1{
			ApplicationID: "store",
			Participants: []app.AppParticipantV1{
				{WalletAddress: userSigner.Address(), SignatureWeight: 1},
				{WalletAddress: appSigner.Address(), SignatureWeight: 1},
			},
			Quorum: 2,
			Nonce:  1,
		},
		IsClosed: false,
		Version:  0,
		Allocations: []app.AppAllocationV1{
			{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
			{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
		},
	}
	sessionCreated := false

	client := &testsupport.FakeClient{
		GetUserAddressFunc: func() string { return userSigner.Address() },
		GetAppsFunc: func(ctx context.Context, opts *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error) {
			return []app.AppInfoV1{{App: app.AppV1{ID: "store", CreationApprovalNotRequired: true}}}, core.PaginationMetadata{}, nil
		},
		CreateAppSessionFunc: func(ctx context.Context, definition app.AppDefinitionV1, sessionData string, quorumSigs []string, opts ...sdk.CreateAppSessionOptions) (string, string, string, error) {
			sessionCreated = true
			currentSession.Version = 1
			currentSession.SessionData = sessionData
			currentSession.AppDefinition = definition
			currentSession.Allocations = []app.AppAllocationV1{
				{Participant: appSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
				{Participant: userSigner.Address(), Asset: "yusd", Amount: decimal.Zero},
			}
			return currentSession.AppSessionID, "1", "open", nil
		},
		GetAppSessionsFunc: func(ctx context.Context, opts *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
			if !sessionCreated {
				return nil, core.PaginationMetadata{}, nil
			}
			if opts != nil && opts.AppSessionID != nil && *opts.AppSessionID != currentSession.AppSessionID {
				return nil, core.PaginationMetadata{}, nil
			}
			return []app.AppSessionInfoV1{currentSession}, core.PaginationMetadata{}, nil
		},
		SubmitAppSessionDepositFunc: func(ctx context.Context, update app.AppStateUpdateV1, quorumSigs []string, asset string, amount decimal.Decimal) (string, error) {
			currentSession.Version = update.Version
			currentSession.SessionData = update.SessionData
			currentSession.Allocations = append([]app.AppAllocationV1(nil), update.Allocations...)
			return "0xnodesig", nil
		},
		SubmitAppStateFunc: func(ctx context.Context, update app.AppStateUpdateV1, quorumSigs []string) error {
			currentSession.Version = update.Version
			currentSession.SessionData = update.SessionData
			currentSession.Allocations = append([]app.AppAllocationV1(nil), update.Allocations...)
			return nil
		},
		GetBalancesFunc: func(ctx context.Context, wallet string) ([]core.BalanceEntry, error) {
			return []core.BalanceEntry{
				{Asset: "yusd", Balance: decimal.RequireFromString("7")},
				{Asset: "yellow", Balance: decimal.RequireFromString("3")},
			}, nil
		},
	}

	return newTestHarnessWithClient(t, client, userSigner, appSigner)
}

func newTestHarness(t *testing.T, client *testsupport.FakeClient) testHarness {
	t.Helper()

	userSigner, err := signing.NewEnvSigner(httpAPITestPrivateKey)
	if err != nil {
		t.Fatalf("NewEnvSigner() error = %v", err)
	}
	appSigner, err := signing.NewStoreAppSigner("", httpAPITestPrivateKey)
	if err != nil {
		t.Fatalf("NewStoreAppSigner() error = %v", err)
	}
	return newTestHarnessWithClient(t, client, userSigner, appSigner)
}

func newTestHarnessWithClient(t *testing.T, client *testsupport.FakeClient, userSigner signing.Signer, appSigner signing.Signer) testHarness {
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
		MerchantName:       "Nitrolite Sandbox Merchant",
		MerchantAppID:      "default",
	}
	if client.GetUserAddressFunc == nil {
		client.GetUserAddressFunc = func() string { return userSigner.Address() }
	}

	manager := nitrolite.NewManagerWithClient(client, nitrolite.Health{
		Connected:     true,
		Ready:         true,
		SignerAddress: userSigner.Address(),
	}, slog.Default())
	appStore, err := store.New(cfg.SQLitePath)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = appStore.Close()
	})
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

func unlockSession(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/unlock", strings.NewReader(`{"api_key":"12345678901234567890123456789012"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unlock status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	return unlockCookie(t, rec.Result().Cookies())
}

func newBrowserSession(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/config", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("store config status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	return browserCookie(t, rec.Result().Cookies())
}

func unlockCookie(t *testing.T, cookies []*http.Cookie) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == writeSessionCookieName {
			return cookie
		}
	}
	t.Fatal("expected write-session cookie")
	return nil
}

func browserCookie(t *testing.T, cookies []*http.Cookie) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == storeBrowserCookieName {
			return cookie
		}
	}
	t.Fatal("expected store browser cookie")
	return nil
}
