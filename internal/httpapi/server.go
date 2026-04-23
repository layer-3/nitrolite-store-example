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

// NewHandler wires the v1 shopper surface for the refreshed store app.
func NewHandler(cfg *config.Config, manager *nitrolite.Manager, _ signing.Signer, appSigner signing.Signer, appStore *store.Store, logger *slog.Logger) (http.Handler, error) {
	ui, err := webui.New()
	if err != nil {
		return nil, err
	}

	authStore := newStoreAuthStore()
	storefront := service.NewWalletStoreService(manager, appStore, appSigner, cfg.StoreName, cfg.StoreAppID, cfg.HomeBlockchains, cfg.ClearnodeWSURL)

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", healthHandler(manager))
	mux.Handle("GET /readyz", readyHandler(manager))
	mux.Handle("POST /api/store/connect/challenge", storeConnectChallengeHandler(authStore))
	mux.Handle("POST /api/store/connect/verify", storeConnectVerifyHandler(authStore))
	mux.Handle("GET /api/store/bootstrap", storeBootstrapHandler(storefront, authStore))
	mux.Handle("POST /api/store/update", storeUpdateHandler(storefront, authStore))
	mux.Handle("GET /api/store/content/{id}", storeContentHandler(storefront, authStore))
	mux.Handle("/", ui)

	return recoveryMiddleware(logger, loggingMiddleware(logger, mux)), nil
}
