package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/layer-3/nitrolite-go-example/internal/config"
	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-go-example/internal/service"
	"github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite-go-example/internal/store"
	"github.com/layer-3/nitrolite-go-example/internal/webui"
)

// NewHandler wires the current HTTP surface.
func NewHandler(cfg *config.Config, manager *nitrolite.Manager, userSigner signing.Signer, appSigner signing.Signer, appStore *store.Store, logger *slog.Logger) (http.Handler, error) {
	ui, err := webui.New()
	if err != nil {
		return nil, err
	}

	nodeService := service.NewNodeService(manager)
	balanceService := service.NewBalanceService(manager)
	channelService := service.NewChannelService(manager)
	mutationService := service.NewMutationService(manager, cfg.HomeBlockchains)
	appSessionService := service.NewAppSessionService(manager, userSigner, nil)
	sessionKeyService := service.NewSessionKeyService(manager, nil)
	demoService := service.NewDemoService(manager, manager.Health, cfg.HomeBlockchains)
	storefrontService := service.NewStorefrontService(manager, appStore, userSigner, appSigner, cfg.StoreName, cfg.StoreAppID, cfg.HomeBlockchains)
	writeSessions := newWriteSessionStore()
	protected := func(handler http.HandlerFunc) http.Handler {
		return requireWriteAccess(cfg.ConsoleAPIKey, writeSessions, handler)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", healthHandler(manager))
	mux.Handle("GET /readyz", readyHandler(manager))
	mux.Handle("GET /openapi.json", openAPIHandler(cfg))
	mux.Handle("GET /api/v1/auth/status", authStatusHandler(cfg.ConsoleAPIKey, writeSessions))
	mux.Handle("POST /api/v1/auth/unlock", authUnlockHandler(cfg.ConsoleAPIKey, writeSessions))
	mux.Handle("POST /api/v1/auth/lock", authLockHandler(writeSessions))
	mux.Handle("GET /api/v1/wallet", walletHandler(cfg, manager))
	mux.Handle("GET /api/v1/node/config", nodeConfigHandler(nodeService))
	mux.Handle("GET /api/v1/node/blockchains", nodeBlockchainsHandler(nodeService))
	mux.Handle("GET /api/v1/node/assets", nodeAssetsHandler(nodeService))
	mux.Handle("GET /api/v1/demo/overview", demoOverviewHandler(demoService))
	mux.Handle("GET /api/v1/balances", balancesHandler(balanceService))
	mux.Handle("GET /api/v1/transactions", transactionsHandler(balanceService))
	mux.Handle("GET /api/v1/channel", channelHandler(channelService))
	mux.Handle("GET /api/v1/channel/state", channelStateHandler(channelService))
	mux.Handle("GET /api/v1/store/config", storeConfigHandler(storefrontService))
	mux.Handle("GET /api/v1/catalog", storeCatalogHandler(storefrontService))
	mux.Handle("GET /api/v1/catalog/{id}", storeCatalogItemHandler(storefrontService))
	mux.Handle("GET /api/v1/store/session", storeSessionHandler(storefrontService))
	mux.Handle("POST /api/v1/store/session/create", createStoreSessionHandler(storefrontService))
	mux.Handle("POST /api/v1/app-session/submit-state", submitStoreStateHandler(storefrontService, cfg.ConsoleAPIKey, writeSessions))
	mux.Handle("GET /api/v1/purchases", purchasesHandlerStore(storefrontService))
	mux.Handle("GET /api/v1/content/{id}", contentHandler(storefrontService))
	mux.Handle("GET /api/v1/balance", storeBalanceHandler(storefrontService))
	mux.Handle("POST /api/v1/approve", protected(approveHandler(mutationService)))
	mux.Handle("POST /api/v1/deposit", protected(depositHandler(mutationService)))
	mux.Handle("POST /api/v1/withdraw", protected(withdrawHandler(mutationService)))
	mux.Handle("POST /api/v1/transfer", protected(transferHandler(mutationService)))
	mux.Handle("POST /api/v1/checkpoint", protected(checkpointHandler(mutationService)))
	mux.Handle("POST /api/v1/channel/close", protected(closeChannelHandler(channelService)))
	mux.Handle("POST /api/v1/challenge", protected(challengeHandler(channelService)))
	mux.Handle("GET /api/v1/apps", appsHandler(appSessionService))
	mux.Handle("POST /api/v1/apps/register", protected(registerAppHandler(appSessionService)))
	mux.Handle("GET /api/v1/sessions", sessionsHandler(appSessionService))
	mux.Handle("GET /api/v1/sessions/{session_id}", sessionDetailHandler(appSessionService))
	mux.Handle("POST /api/v1/sessions", protected(createSessionHandler(appSessionService)))
	mux.Handle("POST /api/v1/sessions/{session_id}/deposit", protected(depositSessionHandler(appSessionService)))
	mux.Handle("POST /api/v1/sessions/{session_id}/state", protected(operateSessionHandler(appSessionService)))
	mux.Handle("POST /api/v1/sessions/{session_id}/close", protected(closeSessionHandler(appSessionService)))
	mux.Handle("GET /api/v1/session-keys/channel", channelSessionKeysHandler(sessionKeyService))
	mux.Handle("POST /api/v1/session-keys/channel", protected(registerChannelSessionKeyHandler(sessionKeyService)))
	mux.Handle("GET /api/v1/session-keys/app", appSessionKeysHandler(sessionKeyService))
	mux.Handle("POST /api/v1/session-keys/app", protected(registerAppSessionKeyHandler(sessionKeyService)))
	mux.Handle("/", ui)

	handler := recoveryMiddleware(logger, loggingMiddleware(logger, mux))
	return handler, nil
}

func walletHandler(cfg *config.Config, manager *nitrolite.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		health := manager.Health()

		writeJSON(w, http.StatusOK, map[string]any{
			"address":         health.SignerAddress,
			"homeBlockchains": cfg.HomeBlockchains,
		})
	}
}
