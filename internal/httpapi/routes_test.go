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
	"time"

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

	cookies := unlockRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected write-session cookie")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/register", strings.NewReader(`{"app_id":"demo-app","metadata":"","creation_approval_not_required":true}`))
	req.AddCookie(cookies[0])
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/lock", strings.NewReader(`{}`))
	lockReq.AddCookie(cookies[0])
	lockRec := httptest.NewRecorder()
	handler.ServeHTTP(lockRec, lockReq)

	if lockRec.Code != http.StatusOK {
		t.Fatalf("lock status = %d, want %d body=%s", lockRec.Code, http.StatusOK, lockRec.Body.String())
	}

	reqAfterLock := httptest.NewRequest(http.MethodPost, "/api/v1/apps/register", strings.NewReader(`{"app_id":"demo-app","metadata":"","creation_approval_not_required":true}`))
	reqAfterLock.AddCookie(cookies[0])
	recAfterLock := httptest.NewRecorder()
	handler.ServeHTTP(recAfterLock, reqAfterLock)

	if recAfterLock.Code != http.StatusUnauthorized {
		t.Fatalf("status after lock = %d, want %d", recAfterLock.Code, http.StatusUnauthorized)
	}
}

func TestAppsEndpoint(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{
		GetAppsFunc: func(ctx context.Context, opts *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error) {
			if opts == nil || opts.Pagination == nil {
				t.Fatalf("opts = %#v", opts)
			}
			return []app.AppInfoV1{
				{
					App: app.AppV1{
						ID:                          "demo-app",
						OwnerWallet:                 "0xabc",
						Metadata:                    "{}",
						Version:                     1,
						CreationApprovalNotRequired: true,
					},
				},
			}, core.PaginationMetadata{Page: 1, PerPage: 10, TotalCount: 1, PageCount: 1}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps?page=1&per_page=10", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		Apps []struct {
			AppID string `json:"app_id"`
		} `json:"apps"`
		Pagination struct {
			Page uint32 `json:"page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(payload.Apps) != 1 || payload.Apps[0].AppID != "demo-app" || payload.Pagination.Page != 1 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCreateSessionEndpoint(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{
		GetAppsFunc: func(ctx context.Context, opts *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error) {
			return []app.AppInfoV1{
				{App: app.AppV1{ID: "demo-app", CreationApprovalNotRequired: true}},
			}, core.PaginationMetadata{}, nil
		},
		CreateAppSessionFunc: func(ctx context.Context, definition app.AppDefinitionV1, sessionData string, quorumSigs []string, opts ...sdk.CreateAppSessionOptions) (string, string, string, error) {
			return "0xsession", "1", "open", nil
		},
		SubmitAppSessionDepositFunc: func(ctx context.Context, update app.AppStateUpdateV1, quorumSigs []string, asset string, amount decimal.Decimal) (string, error) {
			return "0xnodesig", nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(`{"application_id":"demo-app","initial_allocations":[{"asset":"usdc","amount":"5"}],"session_data":"{}"}`))
	req.Header.Set("Authorization", "Bearer 12345678901234567890123456789012")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload struct {
		SessionID string `json:"session_id"`
		Version   uint64 `json:"version"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.SessionID != "0xsession" || payload.Version != 2 || payload.Status != "open" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestOperateSessionEndpointAcceptsSessionData(t *testing.T) {
	t.Parallel()

	var gotUpdate app.AppStateUpdateV1
	handler := newTestHandler(t, &testsupport.FakeClient{
		GetAppSessionsFunc: func(ctx context.Context, opts *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
			return []app.AppSessionInfoV1{
				{
					AppSessionID: "0xsession",
					AppDefinition: app.AppDefinitionV1{
						ApplicationID: "default",
						Participants: []app.AppParticipantV1{
							{WalletAddress: "0xabc", SignatureWeight: 1},
						},
						Quorum: 1,
						Nonce:  1,
					},
					IsClosed:    false,
					Version:     3,
					SessionData: "{}",
					Allocations: []app.AppAllocationV1{
						{Participant: "0xabc", Asset: "usdc", Amount: decimal.RequireFromString("1")},
					},
				},
			}, core.PaginationMetadata{}, nil
		},
		SubmitAppStateFunc: func(ctx context.Context, update app.AppStateUpdateV1, quorumSigs []string) error {
			gotUpdate = update
			return nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/0xsession/state", strings.NewReader(`{"allocations":[{"participant":"0xabc","asset":"usdc","amount":"0.6"}],"session_data":"{\"turn\":1}"}`))
	req.Header.Set("Authorization", "Bearer 12345678901234567890123456789012")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotUpdate.Intent != app.AppStateUpdateIntentOperate {
		t.Fatalf("intent = %s, want %s", gotUpdate.Intent, app.AppStateUpdateIntentOperate)
	}
	if gotUpdate.SessionData != `{"turn":1}` {
		t.Fatalf("session data = %q, want %q", gotUpdate.SessionData, `{"turn":1}`)
	}
}

func TestRegisterChannelSessionKeyRejectsInvalidExpiry(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/session-keys/channel", strings.NewReader(`{"session_key":"0x1111111111111111111111111111111111111111","assets":["usdc"],"expires_at":"not-a-time"}`))
	req.Header.Set("Authorization", "Bearer 12345678901234567890123456789012")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDemoOverviewEndpoint(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &testsupport.FakeClient{
		GetAssetsFunc: func(ctx context.Context, blockchainID *uint64) ([]core.Asset, error) {
			return []core.Asset{
				{
					Name:                  "Yellow USD",
					Symbol:                "yusd",
					Decimals:              6,
					SuggestedBlockchainID: 11155111,
					Tokens: []core.Token{
						{
							Name:         "Yellow USD",
							Symbol:       "YUSD",
							Address:      "0xtoken",
							BlockchainID: 11155111,
							Decimals:     6,
						},
					},
				},
			}, nil
		},
		GetBalancesFunc: func(ctx context.Context, wallet string) ([]core.BalanceEntry, error) {
			return []core.BalanceEntry{{Asset: "yusd", Balance: decimal.RequireFromString("2")}}, nil
		},
		GetHomeChannelFunc: func(ctx context.Context, wallet string, asset string) (*core.Channel, error) {
			return &core.Channel{
				ChannelID:             "0xchannel",
				UserWallet:            wallet,
				Asset:                 asset,
				Type:                  core.ChannelTypeHome,
				BlockchainID:          11155111,
				TokenAddress:          "0xtoken",
				ChallengeDuration:     3600,
				Nonce:                 1,
				ApprovedSigValidators: "0x01",
				Status:                core.ChannelStatusOpen,
				StateVersion:          2,
			}, nil
		},
		GetLatestStateFunc: func(ctx context.Context, wallet string, asset string, onlySigned bool) (*core.State, error) {
			return &core.State{
				ID:         "0xstate",
				Asset:      asset,
				UserWallet: wallet,
				Version:    3,
				Transition: core.Transition{
					Type:   core.TransitionTypeHomeDeposit,
					Amount: decimal.RequireFromString("1"),
				},
				HomeLedger: core.Ledger{
					TokenAddress: "0xtoken",
					BlockchainID: 11155111,
					UserBalance:  decimal.RequireFromString("2"),
					UserNetFlow:  decimal.RequireFromString("2"),
				},
			}, nil
		},
		GetTransactionsFunc: func(ctx context.Context, wallet string, opts *sdk.GetTransactionsOptions) ([]core.Transaction, core.PaginationMetadata, error) {
			return []core.Transaction{
				{
					ID:          "tx-1",
					Asset:       "yusd",
					TxType:      core.TransactionTypeTransfer,
					Amount:      decimal.RequireFromString("1"),
					CreatedAt:   stringsToTime("2026-04-14T00:00:00Z"),
					FromAccount: wallet,
					ToAccount:   "0xnode",
				},
			}, core.PaginationMetadata{}, nil
		},
		GetAppsFunc: func(ctx context.Context, opts *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error) {
			return []app.AppInfoV1{
				{
					App: app.AppV1{
						ID:                          "default",
						CreationApprovalNotRequired: true,
					},
				},
			}, core.PaginationMetadata{}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/demo/overview?asset=yusd", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["selected_asset"] != "yusd" {
		t.Fatalf("selected_asset = %v, want yusd", payload["selected_asset"])
	}
	channelGuidance := payload["channel_guidance"].(map[string]any)
	if channelGuidance["sync_pending"] != true {
		t.Fatalf("sync_pending = %v, want true", channelGuidance["sync_pending"])
	}
}

func TestOpenAPIJSON(t *testing.T) {
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
	if _, ok := paths["/api/v1/auth/unlock"]; !ok {
		t.Fatal("missing /api/v1/auth/unlock path")
	}
	if _, ok := paths["/api/v1/payment-requests"]; !ok {
		t.Fatal("missing /api/v1/payment-requests path")
	}
	if _, ok := paths["/api/v1/operator/lease/acquire"]; !ok {
		t.Fatal("missing /api/v1/operator/lease/acquire path")
	}
}

func TestCreatePaymentRequestRequiresOperatorLease(t *testing.T) {
	t.Parallel()

	harness := newTestHarness(t, &testsupport.FakeClient{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment-requests", strings.NewReader(`{"title":"Sandbox order","description":"demo","asset":"usdc","amount":"1.00"}`))
	req.Header.Set("Authorization", "Bearer 12345678901234567890123456789012")
	rec := httptest.NewRecorder()

	harness.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	unlockReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/unlock", strings.NewReader(`{"api_key":"12345678901234567890123456789012"}`))
	unlockRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(unlockRec, unlockReq)
	if unlockRec.Code != http.StatusOK {
		t.Fatalf("unlock status = %d, want %d body=%s", unlockRec.Code, http.StatusOK, unlockRec.Body.String())
	}
	cookie := unlockRec.Result().Cookies()[0]

	req = httptest.NewRequest(http.MethodPost, "/api/v1/payment-requests", strings.NewReader(`{"title":"Sandbox order","description":"demo","asset":"usdc","amount":"1.00"}`))
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	harness.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status without lease = %d, want %d body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}

	acquireReq := httptest.NewRequest(http.MethodPost, "/api/v1/operator/lease/acquire", strings.NewReader(`{}`))
	acquireReq.AddCookie(cookie)
	acquireRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(acquireRec, acquireReq)
	if acquireRec.Code != http.StatusOK {
		t.Fatalf("acquire status = %d, want %d body=%s", acquireRec.Code, http.StatusOK, acquireRec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/payment-requests", strings.NewReader(`{"title":"Sandbox order","description":"demo","asset":"usdc","amount":"1.00"}`))
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	harness.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var payload struct {
		PaymentRequestID string `json:"payment_request_id"`
		Slug             string `json:"slug"`
		PayURL           string `json:"pay_url"`
		Status           string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.PaymentRequestID == "" || payload.Slug == "" || payload.PayURL == "" || payload.Status != "pending" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestLeaseOwnershipRoutesRejectSecondSession(t *testing.T) {
	t.Parallel()

	harness := newTestHarness(t, &testsupport.FakeClient{})
	cookieA := unlockSession(t, harness.handler)
	cookieB := unlockSession(t, harness.handler)

	acquireReq := httptest.NewRequest(http.MethodPost, "/api/v1/operator/lease/acquire", strings.NewReader(`{}`))
	acquireReq.AddCookie(cookieA)
	acquireRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(acquireRec, acquireReq)
	if acquireRec.Code != http.StatusOK {
		t.Fatalf("acquire status = %d, want %d body=%s", acquireRec.Code, http.StatusOK, acquireRec.Body.String())
	}

	for _, path := range []string{"/api/v1/operator/lease/release", "/api/v1/operator/lease/heartbeat"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		req.AddCookie(cookieB)
		rec := httptest.NewRecorder()
		harness.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s status = %d, want %d body=%s", path, rec.Code, http.StatusConflict, rec.Body.String())
		}
	}
}

func TestPublicPayRouteQueuesOperation(t *testing.T) {
	t.Parallel()

	harness := newTestHarness(t, &testsupport.FakeClient{})
	now := time.Now().UTC()
	if err := harness.store.CreatePaymentRequest(context.Background(), store.PaymentRequest{
		ID:          "req-1",
		Slug:        "merchant-demo",
		Title:       "Sandbox order",
		Description: "demo",
		Asset:       "usdc",
		Amount:      "1.00",
		Status:      "pending",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("CreatePaymentRequest() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment-requests/merchant-demo/pay", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	harness.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var payload struct {
		OperationID string `json:"operation_id"`
		ResourceID  string `json:"resource_id"`
		Status      string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.OperationID == "" || payload.ResourceID == "" || payload.Status != "queued" {
		t.Fatalf("payload = %#v", payload)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/payment-requests/merchant-demo/pay", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	harness.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("repeat status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestDashboardOverviewReturnsServiceUnavailableWhenSDKReadFails(t *testing.T) {
	t.Parallel()

	harness := newTestHarness(t, &testsupport.FakeClient{
		GetAssetsFunc: func(ctx context.Context, blockchainID *uint64) ([]core.Asset, error) {
			return nil, context.DeadlineExceeded
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/overview?asset=usdc", nil)
	rec := httptest.NewRecorder()
	harness.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestAppsEndpointReturnsServiceUnavailableWhenDisconnected(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Port:              "8080",
		LogLevel:          "info",
		ClearnodeWSURL:    "wss://example.invalid",
		DemoPrivateKey:    httpAPITestPrivateKey,
		ConsoleAPIKey:     "12345678901234567890123456789012",
		BlockchainRPCURLs: map[string]string{"80002": "https://example.invalid"},
		HomeBlockchains:   map[string]uint64{"usdc": 80002},
		SQLitePath:        filepath.Join(t.TempDir(), "test.db"),
		MerchantName:      "Nitrolite Sandbox Merchant",
		MerchantAppID:     "default",
	}
	client := &testsupport.FakeClient{
		GetUserAddressFunc: func() string { return "0xabc" },
	}
	manager := nitrolite.NewManagerWithClient(client, nitrolite.Health{
		Connected:     false,
		Ready:         false,
		SignerAddress: "0xabc",
	}, slog.Default())
	signer, err := signing.NewEnvSigner(httpAPITestPrivateKey)
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
	handler, err := NewHandler(cfg, manager, signer, appStore, slog.Default())
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
	handler http.Handler
	store   *store.Store
}

func newTestHarness(t *testing.T, client *testsupport.FakeClient) testHarness {
	t.Helper()

	cfg := &config.Config{
		Port:              "8080",
		LogLevel:          "info",
		ClearnodeWSURL:    "wss://example.invalid",
		DemoPrivateKey:    httpAPITestPrivateKey,
		ConsoleAPIKey:     "12345678901234567890123456789012",
		BlockchainRPCURLs: map[string]string{"80002": "https://example.invalid"},
		HomeBlockchains:   map[string]uint64{"usdc": 80002},
		SQLitePath:        filepath.Join(t.TempDir(), "test.db"),
		MerchantName:      "Nitrolite Sandbox Merchant",
		MerchantAppID:     "default",
	}
	if client.GetUserAddressFunc == nil {
		client.GetUserAddressFunc = func() string { return "0xabc" }
	}

	manager := nitrolite.NewManagerWithClient(client, nitrolite.Health{
		Connected:     true,
		Ready:         true,
		SignerAddress: "0xabc",
	}, slog.Default())
	signer, err := signing.NewEnvSigner(httpAPITestPrivateKey)
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
	handler, err := NewHandler(cfg, manager, signer, appStore, slog.Default())
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return testHarness{handler: handler, store: appStore}
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
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected write-session cookie")
	}
	return cookies[0]
}

func stringsToTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}
