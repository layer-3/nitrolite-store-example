package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/layer-3/nitrolite/pkg/sign"
)

const (
	storeAuthSessionCookieName = "nitrolite_store_auth"
	storeAuthSessionTTL        = 30 * 24 * time.Hour
	storeAuthChallengeTTL      = 5 * time.Minute
)

type storeAuthChallenge struct {
	ID            string
	WalletAddress string
	Message       string
	ExpiresAt     time.Time
}

type storeAuthSession struct {
	Token         string
	WalletAddress string
	ExpiresAt     time.Time
}

var (
	errStoreChallengeNotFound = errors.New("challenge not found")
	errStoreChallengeExpired  = errors.New("challenge expired")
)

type storeAuthStore struct {
	mu         sync.Mutex
	now        func() time.Time
	challenges map[string]storeAuthChallenge
	sessions   map[string]storeAuthSession
}

type storeConnectChallengeRequest struct {
	WalletAddress string `json:"wallet_address"`
}

type storeConnectChallengeResponse struct {
	ChallengeID string `json:"challenge_id"`
	Message     string `json:"message"`
	ExpiresAt   string `json:"expires_at"`
}

type storeConnectVerifyRequest struct {
	ChallengeID   string `json:"challenge_id"`
	WalletAddress string `json:"wallet_address"`
	Signature     string `json:"signature"`
}

type storeConnectVerifyResponse struct {
	WalletAddress string `json:"wallet_address"`
}

func newStoreAuthStore() *storeAuthStore {
	return &storeAuthStore{
		now:        time.Now,
		challenges: make(map[string]storeAuthChallenge),
		sessions:   make(map[string]storeAuthSession),
	}
}

func (s *storeAuthStore) createChallenge(walletAddress string) (*storeAuthChallenge, error) {
	id, err := randomToken()
	if err != nil {
		return nil, err
	}
	expiresAt := s.now().UTC().Add(storeAuthChallengeTTL)
	challenge := storeAuthChallenge{
		ID:            id,
		WalletAddress: walletAddress,
		Message: "Nitrolite Go Example Store\n\n" +
			"Sign this message to use the store.\n" +
			"Wallet: " + walletAddress + "\n" +
			"Nonce: " + id + "\n" +
			"Expires At: " + expiresAt.Format(time.RFC3339),
		ExpiresAt: expiresAt,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.challenges[id] = challenge
	return &challenge, nil
}

func (s *storeAuthStore) consumeChallenge(id string) (*storeAuthChallenge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	challenge, ok := s.challenges[id]
	if !ok {
		return nil, errStoreChallengeNotFound
	}
	delete(s.challenges, id)
	if !challenge.ExpiresAt.After(s.now().UTC()) {
		return nil, errStoreChallengeExpired
	}
	return &challenge, nil
}

func (s *storeAuthStore) createSession(walletAddress string) (*storeAuthSession, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	session := storeAuthSession{
		Token:         token,
		WalletAddress: walletAddress,
		ExpiresAt:     s.now().UTC().Add(storeAuthSessionTTL),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = session
	return &session, nil
}

func (s *storeAuthStore) walletForToken(token string) (string, bool) {
	if strings.TrimSpace(token) == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[token]
	if !ok {
		return "", false
	}
	if !session.ExpiresAt.After(s.now().UTC()) {
		delete(s.sessions, token)
		return "", false
	}
	return session.WalletAddress, true
}

func storeConnectChallengeHandler(auth *storeAuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req storeConnectChallengeRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		walletAddress := strings.TrimSpace(req.WalletAddress)
		if walletAddress == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "wallet_address is required")
			return
		}

		challenge, err := auth.createChallenge(walletAddress)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to create challenge")
			return
		}

		writeJSON(w, http.StatusOK, storeConnectChallengeResponse{
			ChallengeID: challenge.ID,
			Message:     challenge.Message,
			ExpiresAt:   challenge.ExpiresAt.Format(time.RFC3339),
		})
	}
}

func storeConnectVerifyHandler(auth *storeAuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req storeConnectVerifyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		challenge, err := auth.consumeChallenge(strings.TrimSpace(req.ChallengeID))
		if err != nil {
			switch err {
			case errStoreChallengeNotFound:
				writeError(w, http.StatusNotFound, "not_found", err.Error())
			case errStoreChallengeExpired:
				writeError(w, http.StatusConflict, "conflict", err.Error())
			default:
				writeError(w, http.StatusInternalServerError, "internal_error", "failed to verify challenge")
			}
			return
		}
		if !strings.EqualFold(strings.TrimSpace(req.WalletAddress), challenge.WalletAddress) {
			writeError(w, http.StatusConflict, "conflict", "wallet_address does not match challenge")
			return
		}

		signature, err := hexutil.Decode(strings.TrimSpace(req.Signature))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid signature")
			return
		}
		recoverer, err := sign.NewAddressRecoverer(sign.TypeEthereumMsg)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to initialize signature verifier")
			return
		}
		address, err := recoverer.RecoverAddress([]byte(challenge.Message), signature)
		if err != nil || !strings.EqualFold(address.String(), challenge.WalletAddress) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "signature does not match wallet")
			return
		}

		session, err := auth.createSession(challenge.WalletAddress)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to create auth session")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     storeAuthSessionCookieName,
			Value:    session.Token,
			Path:     "/",
			Expires:  session.ExpiresAt,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   requestIsSecure(r),
		})

		writeJSON(w, http.StatusOK, storeConnectVerifyResponse{WalletAddress: session.WalletAddress})
	}
}

func requireStoreAuth(auth *storeAuthStore, next func(http.ResponseWriter, *http.Request, string)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(storeAuthSessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "connect your wallet first")
			return
		}
		walletAddress, ok := auth.walletForToken(cookie.Value)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "connect your wallet first")
			return
		}
		next(w, r, walletAddress)
	})
}
