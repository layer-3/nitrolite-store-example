package httpapi

import (
	"net/http"

	"github.com/layer-3/nitrolite-go-example/internal/service"
)

func storeBootstrapHandler(storefront *service.WalletStoreService, auth *storeAuthStore) http.Handler {
	return requireStoreAuth(auth, func(w http.ResponseWriter, r *http.Request, walletAddress string) {
		bootstrap, err := storefront.Bootstrap(r.Context(), walletAddress, r.URL.Query().Get("asset"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, bootstrap)
	})
}

func storeUpdateHandler(storefront *service.WalletStoreService, auth *storeAuthStore) http.Handler {
	return requireStoreAuth(auth, func(w http.ResponseWriter, r *http.Request, walletAddress string) {
		var req service.StoreUpdateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		bootstrap, err := storefront.SubmitUpdate(r.Context(), walletAddress, req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, bootstrap)
	})
}

func storeContentHandler(storefront *service.WalletStoreService, auth *storeAuthStore) http.Handler {
	return requireStoreAuth(auth, func(w http.ResponseWriter, r *http.Request, walletAddress string) {
		item, err := storefront.Content(r.Context(), walletAddress, r.URL.Query().Get("asset"), r.PathValue("id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}
