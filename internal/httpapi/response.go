package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/layer-3/nitrolite-go-example/internal/service"
)

type errorEnvelope struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, errorEnvelope{
		Error: apiError{
			Code:    code,
			Message: message,
		},
	})
}

func writeServiceError(w http.ResponseWriter, err error) {
	var validationErr service.ValidationError
	if errors.As(err, &validationErr) {
		writeError(w, http.StatusBadRequest, "invalid_request", validationErr.Error())
		return
	}

	var notFoundErr service.NotFoundError
	if errors.As(err, &notFoundErr) {
		writeError(w, http.StatusUnprocessableEntity, "not_found", notFoundErr.Error())
		return
	}

	var conflictErr service.ConflictError
	if errors.As(err, &conflictErr) {
		writeError(w, http.StatusConflict, "conflict", conflictErr.Error())
		return
	}

	if errors.Is(err, service.ErrUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "clearnode not reachable")
		return
	}
	writeError(w, http.StatusUnprocessableEntity, "sdk_operation_failed", err.Error())
}
