package httpapi

import (
	"net/http"

	"github.com/layer-3/nitrolite-go-example/internal/service"
)

func nodeConfigHandler(nodeService *service.NodeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := nodeService.GetConfig(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, encodeNodeConfig(cfg))
	}
}

func nodeBlockchainsHandler(nodeService *service.NodeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		blockchains, err := nodeService.GetBlockchains(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"blockchains": encodeBlockchains(blockchains),
		})
	}
}

func nodeAssetsHandler(nodeService *service.NodeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		blockchainID, err := optionalUint64Query(r, "blockchain_id")
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		assets, err := nodeService.GetAssets(r.Context(), blockchainID)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"assets": encodeAssets(assets),
		})
	}
}
