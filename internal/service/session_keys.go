package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
	sdk "github.com/layer-3/nitrolite/sdk/go"
)

// NowFunc provides testable current-time injection.
type NowFunc func() time.Time

// RegisterChannelSessionKeyRequest contains inputs for channel key registration.
type RegisterChannelSessionKeyRequest struct {
	SessionKey string
	Assets     []string
	ExpiresAt  time.Time
}

// RegisterAppSessionKeyRequest contains inputs for app key registration.
type RegisterAppSessionKeyRequest struct {
	SessionKey     string
	ApplicationIDs []string
	AppSessionIDs  []string
	ExpiresAt      time.Time
}

// SessionKeyService exposes session-key registration and listing.
type SessionKeyService struct {
	provider clientProvider
	now      NowFunc
}

// NewSessionKeyService constructs a SessionKeyService.
func NewSessionKeyService(provider clientProvider, now NowFunc) *SessionKeyService {
	if now == nil {
		now = time.Now
	}
	return &SessionKeyService{
		provider: provider,
		now:      now,
	}
}

// RegisterChannelKey registers a channel session key state.
func (s *SessionKeyService) RegisterChannelKey(ctx context.Context, req RegisterChannelSessionKeyRequest) (*core.ChannelSessionKeyStateV1, error) {
	client, signerAddress, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	sessionKey, err := normalizeAddress(req.SessionKey)
	if err != nil {
		return nil, err
	}
	assets, err := normalizeAssets(req.Assets)
	if err != nil {
		return nil, err
	}
	if !req.ExpiresAt.After(s.now()) {
		return nil, invalidf("expires_at must be in the future")
	}

	existing, err := client.GetLastChannelKeyStates(ctx, signerAddress, &sdk.GetLastChannelKeyStatesOptions{
		SessionKey: &sessionKey,
	})
	if err != nil {
		return nil, err
	}

	state := core.ChannelSessionKeyStateV1{
		UserAddress: signerAddress,
		SessionKey:  sessionKey,
		Version:     nextChannelSessionKeyVersion(existing, sessionKey),
		Assets:      assets,
		ExpiresAt:   req.ExpiresAt.UTC(),
	}
	sig, err := client.SignChannelSessionKeyState(state)
	if err != nil {
		return nil, err
	}
	state.UserSig = sig
	if err := client.SubmitChannelSessionKeyState(ctx, state); err != nil {
		return nil, err
	}
	return &state, nil
}

// GetChannelKeys lists channel session key states.
func (s *SessionKeyService) GetChannelKeys(ctx context.Context, sessionKey *string) ([]core.ChannelSessionKeyStateV1, error) {
	client, signerAddress, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	var opts *sdk.GetLastChannelKeyStatesOptions
	if sessionKey != nil {
		normalized, err := normalizeAddress(*sessionKey)
		if err != nil {
			return nil, err
		}
		opts = &sdk.GetLastChannelKeyStatesOptions{SessionKey: &normalized}
	}

	states, err := client.GetLastChannelKeyStates(ctx, signerAddress, opts)
	if err != nil {
		return nil, err
	}
	return states, nil
}

// RegisterAppKey registers an app session key state.
func (s *SessionKeyService) RegisterAppKey(ctx context.Context, req RegisterAppSessionKeyRequest) (*app.AppSessionKeyStateV1, error) {
	client, signerAddress, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	sessionKey, err := normalizeAddress(req.SessionKey)
	if err != nil {
		return nil, err
	}
	applicationIDs := normalizeStringList(req.ApplicationIDs)
	appSessionIDs := normalizeStringList(req.AppSessionIDs)
	if len(applicationIDs) == 0 && len(appSessionIDs) == 0 {
		return nil, invalidf("application_ids or app_session_ids must not be empty")
	}
	if !req.ExpiresAt.After(s.now()) {
		return nil, invalidf("expires_at must be in the future")
	}

	existing, err := client.GetLastAppKeyStates(ctx, signerAddress, &sdk.GetLastKeyStatesOptions{
		SessionKey: &sessionKey,
	})
	if err != nil {
		return nil, err
	}

	state := app.AppSessionKeyStateV1{
		UserAddress:    signerAddress,
		SessionKey:     sessionKey,
		Version:        nextAppSessionKeyVersion(existing, sessionKey),
		ApplicationIDs: applicationIDs,
		AppSessionIDs:  appSessionIDs,
		ExpiresAt:      req.ExpiresAt.UTC(),
	}
	sig, err := client.SignSessionKeyState(state)
	if err != nil {
		return nil, err
	}
	state.UserSig = sig
	if err := client.SubmitAppSessionKeyState(ctx, state); err != nil {
		return nil, err
	}
	return &state, nil
}

// GetAppKeys lists app session key states.
func (s *SessionKeyService) GetAppKeys(ctx context.Context, sessionKey *string) ([]app.AppSessionKeyStateV1, error) {
	client, signerAddress, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	var opts *sdk.GetLastKeyStatesOptions
	if sessionKey != nil {
		normalized, err := normalizeAddress(*sessionKey)
		if err != nil {
			return nil, err
		}
		opts = &sdk.GetLastKeyStatesOptions{SessionKey: &normalized}
	}

	states, err := client.GetLastAppKeyStates(ctx, signerAddress, opts)
	if err != nil {
		return nil, err
	}
	return states, nil
}

func normalizeAddress(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", invalidf("session_key is required")
	}
	if !common.IsHexAddress(trimmed) {
		return "", invalidf("invalid session_key")
	}
	return strings.ToLower(trimmed), nil
}

func normalizeAssets(assets []string) ([]string, error) {
	normalized := normalizeStringList(assets)
	if len(normalized) == 0 {
		return nil, invalidf("assets must not be empty")
	}
	for i := range normalized {
		normalized[i] = strings.ToLower(normalized[i])
	}
	sort.Strings(normalized)
	return uniqueStrings(normalized), nil
}

func normalizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return uniqueStrings(out)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := make([]string, 0, len(values))
	var prev string
	for i, value := range values {
		if i == 0 || value != prev {
			out = append(out, value)
		}
		prev = value
	}
	return out
}

func nextChannelSessionKeyVersion(existing []core.ChannelSessionKeyStateV1, sessionKey string) uint64 {
	var max uint64
	for _, state := range existing {
		if strings.EqualFold(state.SessionKey, sessionKey) && state.Version > max {
			max = state.Version
		}
	}
	return max + 1
}

func nextAppSessionKeyVersion(existing []app.AppSessionKeyStateV1, sessionKey string) uint64 {
	var max uint64
	for _, state := range existing {
		if strings.EqualFold(state.SessionKey, sessionKey) && state.Version > max {
			max = state.Version
		}
	}
	return max + 1
}
