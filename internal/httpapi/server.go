package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/layer-3/nitrolite-go-example/internal/config"
	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-go-example/internal/service"
	"github.com/layer-3/nitrolite-go-example/internal/webui"
)

// NewHandler wires the current HTTP surface.
func NewHandler(cfg *config.Config, manager *nitrolite.Manager, logger *slog.Logger) (http.Handler, error) {
	ui, err := webui.New()
	if err != nil {
		return nil, err
	}

	nodeService := service.NewNodeService(manager)
	balanceService := service.NewBalanceService(manager)
	channelService := service.NewChannelService(manager)

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", healthHandler(manager))
	mux.Handle("GET /readyz", readyHandler(manager))
	mux.Handle("GET /api/v1/wallet", walletHandler(cfg, manager))
	mux.Handle("GET /api/v1/node/config", nodeConfigHandler(nodeService))
	mux.Handle("GET /api/v1/node/blockchains", nodeBlockchainsHandler(nodeService))
	mux.Handle("GET /api/v1/node/assets", nodeAssetsHandler(nodeService))
	mux.Handle("GET /api/v1/balances", balancesHandler(balanceService))
	mux.Handle("GET /api/v1/transactions", transactionsHandler(balanceService))
	mux.Handle("GET /api/v1/channel", channelHandler(channelService))
	mux.Handle("GET /api/v1/channel/state", channelStateHandler(channelService))
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
