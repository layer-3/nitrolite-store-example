package httpapi

import (
	"net/http"

	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
)

func healthHandler(manager *nitrolite.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		health := manager.Health()
		clearnode := "disconnected"
		if health.Connected {
			clearnode = "connected"
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"clearnode": clearnode,
			"signer":    health.SignerAddress,
		})
	}
}

func readyHandler(manager *nitrolite.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		health := manager.Health()
		if !health.Ready {
			writeError(w, http.StatusServiceUnavailable, "not_ready", "service not ready")
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}
