package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/layer-3/nitrolite-store-example/internal/config"
	"github.com/layer-3/nitrolite-store-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-store-example/internal/service"
	"github.com/layer-3/nitrolite-store-example/internal/signing"
	"github.com/layer-3/nitrolite-store-example/internal/store"
	"github.com/layer-3/nitrolite-store-example/internal/webui"
)

// NewHandler wires the v1 shopper surface for the refreshed store app.
func NewHandler(cfg *config.Config, manager *nitrolite.Manager, _ signing.Signer, appSigner signing.Signer, appStore *store.Store, logger *slog.Logger) (http.Handler, error) {
	ui, err := webui.New()
	if err != nil {
		return nil, err
	}

	storefront := service.NewWalletStoreService(manager, appStore, appSigner, cfg.StoreName, cfg.StoreAppID, cfg.HomeBlockchains, cfg.ClearnodeWSURL)

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", healthHandler(manager))
	mux.Handle("GET /readyz", readyHandler(manager))
	mux.Handle("GET /api/store/bootstrap", storeBootstrapHandler(storefront))
	mux.Handle("POST /api/store/init", storeInitHandler(storefront))
	mux.Handle("POST /api/store/update", storeUpdateHandler(storefront))
	mux.Handle("GET /api/store/content/{id}", storeContentLegacyHandler())
	mux.Handle("POST /api/store/content/{id}/open", storeContentHandler(storefront))
	mux.Handle("/", ui)

	return recoveryMiddleware(logger, loggingMiddleware(logger, mux)), nil
}
