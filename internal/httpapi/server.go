package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/layer-3/nitrolite-go-example/internal/config"
	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-go-example/internal/webui"
)

// NewHandler wires the current HTTP surface.
func NewHandler(cfg *config.Config, manager *nitrolite.Manager, logger *slog.Logger) (http.Handler, error) {
	ui, err := webui.New()
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", healthHandler(manager))
	mux.Handle("GET /readyz", readyHandler(manager))
	mux.Handle("GET /api/v1/wallet", walletHandler(cfg, manager))
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
