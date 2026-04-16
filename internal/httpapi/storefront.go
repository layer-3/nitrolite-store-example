package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/layer-3/nitrolite-go-example/internal/service"
)

type storeAssetRequest struct {
	Asset string `json:"asset"`
}

func storeConfigHandler(storefront *service.StorefrontService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		browserID, _, err := ensureStoreBrowserSession(w, r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to create browser session")
			return
		}
		payload := storefront.Config()
		writeJSON(w, http.StatusOK, map[string]any{
			"browser_session_id": browserID,
			"store":              payload,
		})
	}
}

func storeCatalogHandler(storefront *service.StorefrontService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := storefront.Catalog(r.URL.Query().Get("asset"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func storeCatalogItemHandler(storefront *service.StorefrontService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := storefront.CatalogItem(r.URL.Query().Get("asset"), r.PathValue("id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": item})
	}
}

func storeSessionHandler(storefront *service.StorefrontService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		browserID, _, err := ensureStoreBrowserSession(w, r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to create browser session")
			return
		}
		summary, err := storefront.GetSession(r.Context(), browserID, r.URL.Query().Get("asset"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"session": summary})
	}
}

func createStoreSessionHandler(storefront *service.StorefrontService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		browserID, _, err := ensureStoreBrowserSession(w, r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to create browser session")
			return
		}

		var req storeAssetRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		summary, err := storefront.CreateSession(r.Context(), browserID, req.Asset)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"session": summary})
	}
}

func submitStoreStateHandler(storefront *service.StorefrontService, expectedAPIKey string, sessions *writeSessionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		browserID, _, err := ensureStoreBrowserSession(w, r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to create browser session")
			return
		}

		var req service.StoreStateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		allowAppWithdraw := hasValidBearerKey(r, expectedAPIKey)
		if !allowAppWithdraw {
			if token, ok := readWriteSession(r); ok {
				_, allowAppWithdraw = sessions.Validate(token)
			}
		}

		summary, err := storefront.SubmitState(r.Context(), browserID, req, allowAppWithdraw)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"session": summary})
	}
}

func purchasesHandlerStore(storefront *service.StorefrontService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		browserID, ok := readStoreBrowserSession(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing browser session")
			return
		}
		summary, err := storefront.GetSession(r.Context(), browserID, r.URL.Query().Get("asset"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"purchases": summary.Purchases})
	}
}

func contentHandler(storefront *service.StorefrontService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		browserID, ok := readStoreBrowserSession(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing browser session")
			return
		}
		item, err := storefront.Content(r.Context(), browserID, r.URL.Query().Get("asset"), r.PathValue("id"))
		if err != nil {
			var conflictErr service.ConflictError
			if errors.As(err, &conflictErr) || strings.Contains(strings.ToLower(err.Error()), "not purchased") {
				writeError(w, http.StatusForbidden, "forbidden", "item not purchased")
				return
			}
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": item})
	}
}

func storeBalanceHandler(storefront *service.StorefrontService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		browserID, ok := readStoreBrowserSession(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing browser session")
			return
		}
		summary, err := storefront.GetSession(r.Context(), browserID, r.URL.Query().Get("asset"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"asset":             summary.Asset,
			"available_balance": summary.AvailableBalance,
			"user_allocation":   summary.UserAllocation,
			"app_allocation":    summary.AppAllocation,
			"session_status":    summary.Status,
			"session_id":        summary.AppSessionID,
			"version":           summary.Version,
		})
	}
}
