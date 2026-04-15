package httpapi

import (
	"net/http"
	"time"

	"github.com/layer-3/nitrolite-go-example/internal/service"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
)

type registerChannelSessionKeyRequest struct {
	SessionKey string   `json:"session_key"`
	Assets     []string `json:"assets"`
	ExpiresAt  string   `json:"expires_at"`
}

type registerAppSessionKeyRequest struct {
	SessionKey     string   `json:"session_key"`
	ApplicationIDs []string `json:"application_ids"`
	AppSessionIDs  []string `json:"app_session_ids"`
	ExpiresAt      string   `json:"expires_at"`
}

func channelSessionKeysHandler(sessionKeys *service.SessionKeyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		states, err := sessionKeys.GetChannelKeys(r.Context(), optionalQuery(r, "session_key"))
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"states": encodeChannelSessionKeyStates(states),
		})
	}
}

func registerChannelSessionKeyHandler(sessionKeys *service.SessionKeyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerChannelSessionKeyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid expires_at")
			return
		}

		state, err := sessionKeys.RegisterChannelKey(r.Context(), service.RegisterChannelSessionKeyRequest{
			SessionKey: req.SessionKey,
			Assets:     req.Assets,
			ExpiresAt:  expiresAt,
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"state": encodeChannelSessionKeyStates([]core.ChannelSessionKeyStateV1{*state})[0],
		})
	}
}

func appSessionKeysHandler(sessionKeys *service.SessionKeyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		states, err := sessionKeys.GetAppKeys(r.Context(), optionalQuery(r, "session_key"))
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"states": encodeAppSessionKeyStates(states),
		})
	}
}

func registerAppSessionKeyHandler(sessionKeys *service.SessionKeyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerAppSessionKeyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid expires_at")
			return
		}

		state, err := sessionKeys.RegisterAppKey(r.Context(), service.RegisterAppSessionKeyRequest{
			SessionKey:     req.SessionKey,
			ApplicationIDs: req.ApplicationIDs,
			AppSessionIDs:  req.AppSessionIDs,
			ExpiresAt:      expiresAt,
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"state": encodeAppSessionKeyStates([]app.AppSessionKeyStateV1{*state})[0],
		})
	}
}
