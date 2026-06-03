package httpapi

import (
	"net/http"
	"strings"

	"github.com/layer-3/nitrolite-store-example/internal/service"
)

func storeBootstrapHandler(storefront *service.WalletStoreService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		walletAddress := strings.TrimSpace(r.URL.Query().Get("wallet_address"))
		if walletAddress == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "wallet_address is required")
			return
		}

		bootstrap, err := storefront.Bootstrap(r.Context(), walletAddress, r.URL.Query().Get("asset"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, bootstrap)
	})
}

func storeInitHandler(storefront *service.WalletStoreService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req service.StoreInitRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		bootstrap, err := storefront.CreateSession(r.Context(), req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, bootstrap)
	})
}

func storeUpdateHandler(storefront *service.WalletStoreService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req service.StoreUpdateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		result, err := storefront.SubmitUpdate(r.Context(), req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

func storeContentHandler(storefront *service.WalletStoreService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		item, err := storefront.Content(r.Context(), r.PathValue("id"), service.StoreContentRequest{
			WalletAddress: r.URL.Query().Get("wallet_address"),
			Asset:         r.URL.Query().Get("asset"),
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func storeContentSignedPostHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "content reads use GET /api/store/content/{id}?wallet_address=...&asset=...")
	})
}
