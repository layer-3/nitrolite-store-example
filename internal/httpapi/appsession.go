package httpapi

import (
	"net/http"

	"github.com/layer-3/nitrolite-go-example/internal/service"
)

type registerAppRequest struct {
	AppID                       string `json:"app_id"`
	Metadata                    string `json:"metadata"`
	CreationApprovalNotRequired bool   `json:"creation_approval_not_required"`
}

type createSessionRequest struct {
	ApplicationID      string                           `json:"application_id"`
	InitialAllocations []service.InitialAllocationInput `json:"initial_allocations"`
	SessionData        string                           `json:"session_data"`
}

func appsHandler(appSessions *service.AppSessionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := positiveUint32Query(r, "page", 1)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		perPage, err := positiveUint32Query(r, "per_page", 20)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		apps, meta, err := appSessions.GetApps(r.Context(), service.AppListFilter{
			AppID:       optionalQuery(r, "app_id"),
			OwnerWallet: optionalQuery(r, "owner_wallet"),
			Page:        page,
			PerPage:     perPage,
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"apps":       encodeApps(apps),
			"pagination": encodePagination(meta),
		})
	}
}

func registerAppHandler(appSessions *service.AppSessionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerAppRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		if err := appSessions.RegisterApp(r.Context(), req.AppID, req.Metadata, req.CreationApprovalNotRequired); err != nil {
			writeServiceError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func sessionsHandler(appSessions *service.AppSessionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := positiveUint32Query(r, "page", 1)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		perPage, err := positiveUint32Query(r, "per_page", 20)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		sessions, meta, err := appSessions.GetSessions(r.Context(), service.SessionListFilter{
			Status:       r.URL.Query().Get("status"),
			AppSessionID: optionalQuery(r, "app_session_id"),
			Participant:  optionalQuery(r, "participant"),
			Page:         page,
			PerPage:      perPage,
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}

		encoded := make([]appSessionResponse, 0, len(sessions))
		for _, session := range sessions {
			encoded = append(encoded, encodeAppSession(session))
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"sessions":   encoded,
			"pagination": encodePagination(meta),
		})
	}
}

func sessionDetailHandler(appSessions *service.AppSessionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		detail, err := appSessions.GetSession(r.Context(), r.PathValue("session_id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"session":        encodeAppSession(detail.Session),
			"app_definition": encodeAppDefinition(detail.AppDefinition),
		})
	}
}

func createSessionHandler(appSessions *service.AppSessionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createSessionRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		created, err := appSessions.CreateSession(r.Context(), service.CreateAppSessionRequest{
			ApplicationID:      req.ApplicationID,
			InitialAllocations: req.InitialAllocations,
			SessionData:        req.SessionData,
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"session_id": created.SessionID,
			"version":    created.Version,
			"status":     created.Status,
		})
	}
}

func depositSessionHandler(appSessions *service.AppSessionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req service.DepositAppSessionRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		deposited, err := appSessions.DepositSession(r.Context(), r.PathValue("session_id"), req)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"session_id": deposited.SessionID,
			"version":    deposited.Version,
			"node_sig":   deposited.NodeSig,
		})
	}
}

func operateSessionHandler(appSessions *service.AppSessionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req service.OperateAppSessionRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		updated, err := appSessions.OperateSession(r.Context(), r.PathValue("session_id"), req)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"session_id": updated.SessionID,
			"version":    updated.Version,
		})
	}
}

func closeSessionHandler(appSessions *service.AppSessionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct{}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		closed, err := appSessions.CloseSession(r.Context(), r.PathValue("session_id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"session_id":        closed.SessionID,
			"status":            closed.Status,
			"final_allocations": encodeAppAllocations(closed.FinalAllocations),
		})
	}
}
