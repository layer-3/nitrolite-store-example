package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/layer-3/nitrolite-go-example/internal/store"
)

const (
	writeSessionCookieName = "nitrolite_write_session"
	writeSessionTTL        = 2 * time.Hour
)

type unlockRequest struct {
	APIKey string `json:"api_key"`
}

type authStatusResponse struct {
	Unlocked  bool   `json:"unlocked"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type writeSessionStore struct {
	mu       sync.Mutex
	sessions map[string]time.Time
	now      func() time.Time
}

func newWriteSessionStore() *writeSessionStore {
	return &writeSessionStore{
		sessions: make(map[string]time.Time),
		now:      time.Now,
	}
}

func (s *writeSessionStore) Create() (string, time.Time, error) {
	token, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := s.now().UTC().Add(writeSessionTTL)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = expiresAt
	return token, expiresAt, nil
}

func (s *writeSessionStore) Validate(token string) (time.Time, bool) {
	if token == "" {
		return time.Time{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	expiresAt, ok := s.sessions[token]
	if !ok {
		return time.Time{}, false
	}
	if !expiresAt.After(s.now().UTC()) {
		delete(s.sessions, token)
		return time.Time{}, false
	}
	return expiresAt, true
}

func (s *writeSessionStore) Delete(token string) {
	if token == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func authUnlockHandler(expectedAPIKey string, sessions *writeSessionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req unlockRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if strings.TrimSpace(req.APIKey) == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "api_key is required")
			return
		}
		if strings.TrimSpace(req.APIKey) != expectedAPIKey {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid api key")
			return
		}

		token, expiresAt, err := sessions.Create()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to create write session")
			return
		}

		http.SetCookie(w, newWriteSessionCookie(token, expiresAt, requestIsSecure(r)))
		writeJSON(w, http.StatusOK, authStatusResponse{
			Unlocked:  true,
			ExpiresAt: expiresAt.Format(time.RFC3339),
		})
	}
}

func authLockHandler(sessions *writeSessionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, _ := readWriteSession(r)
		sessions.Delete(token)

		http.SetCookie(w, &http.Cookie{
			Name:     writeSessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
			SameSite: http.SameSiteLaxMode,
			Secure:   requestIsSecure(r),
		})

		writeJSON(w, http.StatusOK, authStatusResponse{Unlocked: false})
	}
}

func authStatusHandler(expectedAPIKey string, sessions *writeSessionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if hasValidBearerKey(r, expectedAPIKey) {
			writeJSON(w, http.StatusOK, authStatusResponse{Unlocked: true})
			return
		}

		token, ok := readWriteSession(r)
		if !ok {
			writeJSON(w, http.StatusOK, authStatusResponse{Unlocked: false})
			return
		}

		expiresAt, ok := sessions.Validate(token)
		if !ok {
			writeJSON(w, http.StatusOK, authStatusResponse{Unlocked: false})
			return
		}

		writeJSON(w, http.StatusOK, authStatusResponse{
			Unlocked:  true,
			ExpiresAt: expiresAt.Format(time.RFC3339),
		})
	}
}

func requireWriteAccess(expectedAPIKey string, sessions *writeSessionStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hasValidBearerKey(r, expectedAPIKey) {
			next.ServeHTTP(w, r)
			return
		}

		token, ok := readWriteSession(r)
		if ok {
			if _, valid := sessions.Validate(token); valid {
				next.ServeHTTP(w, r)
				return
			}
		}

		writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid api key")
	})
}

func requireLeaseOwnership(sessions *writeSessionStore, leases *store.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := readWriteSession(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing write session")
			return
		}
		lease, err := leases.GetLease(r.Context())
		if err != nil {
			if err == store.ErrNotFound {
				writeError(w, http.StatusConflict, "conflict", "operator lease not held")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to read operator lease")
			return
		}
		if lease.SessionToken != token {
			writeError(w, http.StatusConflict, "conflict", "operator lease held by another session")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func requireOperatorLease(expectedAPIKey string, sessions *writeSessionStore, leases *store.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := readWriteSession(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing write session")
			return
		}
		if _, valid := sessions.Validate(token); !valid {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid api key")
			return
		}

		lease, err := leases.ActiveLeaseForSession(r.Context(), token)
		if err != nil {
			switch err {
			case store.ErrNotFound:
				writeError(w, http.StatusConflict, "conflict", "operator lease held by another session or expired")
			default:
				writeError(w, http.StatusInternalServerError, "internal_error", "failed to read operator lease")
			}
			return
		}
		if lease == nil {
			writeError(w, http.StatusConflict, "conflict", "operator lease not held")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func hasValidBearerKey(r *http.Request, expected string) bool {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	return token != "" && token == expected
}

func readWriteSession(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(writeSessionCookieName)
	if err != nil {
		return "", false
	}
	token := strings.TrimSpace(cookie.Value)
	return token, token != ""
}

func newWriteSessionCookie(token string, expiresAt time.Time, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     writeSessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}
}

func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

var errNoCookie = errors.New("missing cookie")
