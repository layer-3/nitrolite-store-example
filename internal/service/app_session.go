package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsigning "github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
	sdk "github.com/layer-3/nitrolite/sdk/go"
	"github.com/shopspring/decimal"
)

// NonceSource provides deterministic nonce generation for app session creation.
type NonceSource func() uint64

// AppListFilter controls app list queries.
type AppListFilter struct {
	AppID       *string
	OwnerWallet *string
	Page        uint32
	PerPage     uint32
}

// SessionListFilter controls app session list queries.
type SessionListFilter struct {
	Status       string
	AppSessionID *string
	Participant  *string
	Page         uint32
	PerPage      uint32
}

// InitialAllocationInput represents a minimal create-session allocation.
type InitialAllocationInput struct {
	Asset  string          `json:"asset"`
	Amount decimal.Decimal `json:"amount"`
}

// AllocationInput represents an app-session allocation input.
type AllocationInput struct {
	Participant string          `json:"participant"`
	Asset       string          `json:"asset"`
	Amount      decimal.Decimal `json:"amount"`
}

// CreateAppSessionRequest contains create-session inputs.
type CreateAppSessionRequest struct {
	ApplicationID      string                   `json:"application_id"`
	InitialAllocations []InitialAllocationInput `json:"initial_allocations"`
	SessionData        string                   `json:"session_data"`
}

// DepositAppSessionRequest contains session deposit inputs.
type DepositAppSessionRequest struct {
	Asset  string          `json:"asset"`
	Amount decimal.Decimal `json:"amount"`
}

// OperateAppSessionRequest contains operate-session inputs.
type OperateAppSessionRequest struct {
	Allocations []AllocationInput `json:"allocations"`
	SessionData string            `json:"session_data"`
}

// CreatedAppSession captures session creation results.
type CreatedAppSession struct {
	SessionID string
	Version   uint64
	Status    string
}

// DepositedAppSession captures deposit results.
type DepositedAppSession struct {
	SessionID string
	Version   uint64
	NodeSig   string
}

// UpdatedAppSession captures generic update results.
type UpdatedAppSession struct {
	SessionID string
	Version   uint64
}

// ClosedAppSession captures close results.
type ClosedAppSession struct {
	SessionID        string
	Version          uint64
	Status           string
	FinalAllocations []app.AppAllocationV1
}

// AppSessionDetail groups a session with its definition.
type AppSessionDetail struct {
	Session       app.AppSessionInfoV1
	AppDefinition app.AppDefinitionV1
}

// AppSessionService exposes app registry and app session operations.
type AppSessionService struct {
	provider  clientProvider
	signer    appsigning.Signer
	nextNonce NonceSource
}

// NewAppSessionService constructs an AppSessionService.
func NewAppSessionService(provider clientProvider, signer appsigning.Signer, nextNonce NonceSource) *AppSessionService {
	if nextNonce == nil {
		nextNonce = func() uint64 { return uint64(time.Now().UTC().UnixNano()) }
	}
	return &AppSessionService{
		provider:  provider,
		signer:    signer,
		nextNonce: nextNonce,
	}
}

// GetApps lists registered apps.
func (s *AppSessionService) GetApps(ctx context.Context, filter AppListFilter) ([]app.AppInfoV1, core.PaginationMetadata, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, core.PaginationMetadata{}, err
	}

	offset := (filter.Page - 1) * filter.PerPage
	limit := filter.PerPage
	opts := &sdk.GetAppsOptions{
		AppID:       filter.AppID,
		OwnerWallet: filter.OwnerWallet,
		Pagination: &core.PaginationParams{
			Offset: &offset,
			Limit:  &limit,
		},
	}

	apps, meta, err := client.GetApps(ctx, opts)
	if err != nil {
		return nil, core.PaginationMetadata{}, fmt.Errorf("failed to get apps: %w", err)
	}
	return apps, meta, nil
}

// RegisterApp registers a new app.
func (s *AppSessionService) RegisterApp(ctx context.Context, appID string, metadata string, creationApprovalNotRequired bool) error {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return err
	}

	if err := validateAppID(appID); err != nil {
		return err
	}

	if err := client.RegisterApp(ctx, appID, metadata, creationApprovalNotRequired); err != nil {
		return fmt.Errorf("failed to register app: %w", err)
	}
	return nil
}

// GetSessions lists app sessions.
func (s *AppSessionService) GetSessions(ctx context.Context, filter SessionListFilter) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
	client, signerAddress, err := activeClient(s.provider)
	if err != nil {
		return nil, core.PaginationMetadata{}, err
	}

	status, err := normalizeStatusFilter(filter.Status)
	if err != nil {
		return nil, core.PaginationMetadata{}, err
	}

	offset := (filter.Page - 1) * filter.PerPage
	limit := filter.PerPage
	participant := filter.Participant
	if filter.AppSessionID == nil && participant == nil && signerAddress != "" {
		participant = &signerAddress
	}
	opts := &sdk.GetAppSessionsOptions{
		AppSessionID: filter.AppSessionID,
		Participant:  participant,
		Status:       status,
		Pagination: &core.PaginationParams{
			Offset: &offset,
			Limit:  &limit,
		},
	}

	sessions, meta, err := client.GetAppSessions(ctx, opts)
	if err != nil {
		return nil, core.PaginationMetadata{}, fmt.Errorf("failed to get app sessions: %w", err)
	}
	return sessions, meta, nil
}

// GetSession returns a session plus its definition.
func (s *AppSessionService) GetSession(ctx context.Context, sessionID string) (*AppSessionDetail, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	session, err := s.lookupSession(ctx, client, sessionID)
	if err != nil {
		return nil, err
	}
	definition, err := client.GetAppDefinition(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app definition: %w", err)
	}

	return &AppSessionDetail{
		Session:       *session,
		AppDefinition: *definition,
	}, nil
}

// CreateSession creates a single-participant app session for the demo signer.
func (s *AppSessionService) CreateSession(ctx context.Context, req CreateAppSessionRequest) (*CreatedAppSession, error) {
	return s.createSession(ctx, req.ApplicationID, req.InitialAllocations, req.SessionData)
}

// CreateEmptySession creates a session without initial allocations for internal orchestration flows.
func (s *AppSessionService) CreateEmptySession(ctx context.Context, applicationID string, sessionData string) (*CreatedAppSession, error) {
	return s.createSession(ctx, applicationID, nil, sessionData)
}

// DepositSession deposits into an app session.
func (s *AppSessionService) DepositSession(ctx context.Context, sessionID string, req DepositAppSessionRequest) (*DepositedAppSession, error) {
	client, signerAddress, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	current, err := s.lookupSession(ctx, client, sessionID)
	if err != nil {
		return nil, err
	}

	update, err := buildDepositUpdate(*current, signerAddress, req.Asset, req.Amount)
	if err != nil {
		return nil, err
	}
	quorumSig, err := signAppStateUpdate(update, s.signer)
	if err != nil {
		return nil, err
	}

	nodeSig, err := client.SubmitAppSessionDeposit(ctx, update, []string{quorumSig}, strings.ToLower(strings.TrimSpace(req.Asset)), req.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to submit app session deposit: %w", err)
	}

	return &DepositedAppSession{
		SessionID: sessionID,
		Version:   update.Version,
		NodeSig:   nodeSig,
	}, nil
}

// OperateSession submits a generic operate update.
func (s *AppSessionService) OperateSession(ctx context.Context, sessionID string, req OperateAppSessionRequest) (*UpdatedAppSession, error) {
	client, signerAddress, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	current, err := s.lookupSession(ctx, client, sessionID)
	if err != nil {
		return nil, err
	}

	allocations, err := buildOperateAllocations(signerAddress, req.Allocations)
	if err != nil {
		return nil, err
	}
	update, err := buildOperateUpdate(*current, allocations, req.SessionData)
	if err != nil {
		return nil, err
	}
	quorumSig, err := signAppStateUpdate(update, s.signer)
	if err != nil {
		return nil, err
	}

	if err := client.SubmitAppState(ctx, update, []string{quorumSig}); err != nil {
		return nil, fmt.Errorf("failed to submit app state: %w", err)
	}

	return &UpdatedAppSession{
		SessionID: sessionID,
		Version:   update.Version,
	}, nil
}

// CloseSession closes an app session using its current allocations.
func (s *AppSessionService) CloseSession(ctx context.Context, sessionID string) (*ClosedAppSession, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	current, err := s.lookupSession(ctx, client, sessionID)
	if err != nil {
		return nil, err
	}

	update, err := buildCloseUpdate(*current)
	if err != nil {
		return nil, err
	}
	quorumSig, err := signAppStateUpdate(update, s.signer)
	if err != nil {
		return nil, err
	}

	if err := client.SubmitAppState(ctx, update, []string{quorumSig}); err != nil {
		return nil, fmt.Errorf("failed to submit close app state: %w", err)
	}

	return &ClosedAppSession{
		SessionID:        sessionID,
		Version:          update.Version,
		Status:           "closed",
		FinalAllocations: cloneAllocations(current.Allocations),
	}, nil
}

func (s *AppSessionService) createSession(ctx context.Context, applicationID string, allocations []InitialAllocationInput, sessionData string) (*CreatedAppSession, error) {
	client, signerAddress, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	if err := validateAppID(applicationID); err != nil {
		return nil, err
	}

	appInfo, err := s.lookupApp(ctx, client, applicationID)
	if err != nil {
		return nil, err
	}

	definition := buildSingleParticipantAppDefinition(applicationID, signerAddress, s.nextNonce())
	quorumSig, err := signCreateAppSessionRequest(definition, sessionData, s.signer)
	if err != nil {
		return nil, err
	}

	opts := []sdk.CreateAppSessionOptions{}
	if !appInfo.App.CreationApprovalNotRequired {
		ownerSig, err := signCreateAppSessionRequest(definition, sessionData, s.signer)
		if err != nil {
			return nil, fmt.Errorf("failed to sign owner approval: %w", err)
		}
		opts = append(opts, sdk.CreateAppSessionOptions{OwnerSig: ownerSig})
	}

	sessionID, version, status, err := client.CreateAppSession(ctx, definition, sessionData, []string{quorumSig}, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create app session: %w", err)
	}

	parsedVersion, err := parseDecimalVersion(version)
	if err != nil {
		return nil, err
	}
	current := app.AppSessionInfoV1{
		AppSessionID:  sessionID,
		AppDefinition: definition,
		IsClosed:      strings.EqualFold(status, "closed"),
		SessionData:   sessionData,
		Version:       parsedVersion,
		Allocations:   nil,
	}

	if len(allocations) > 0 {
		initialAllocations, err := buildInitialAllocations(signerAddress, allocations)
		if err != nil {
			return nil, err
		}
		for _, allocation := range initialAllocations {
			update, err := buildDepositUpdate(current, signerAddress, allocation.Asset, allocation.Amount)
			if err != nil {
				return nil, err
			}
			quorumSig, err := signAppStateUpdate(update, s.signer)
			if err != nil {
				return nil, err
			}
			if _, err := client.SubmitAppSessionDeposit(ctx, update, []string{quorumSig}, allocation.Asset, allocation.Amount); err != nil {
				return nil, fmt.Errorf("failed to submit initial app session deposit: %w", err)
			}
			current.Version = update.Version
			current.Allocations = cloneAllocations(update.Allocations)
		}
	}

	return &CreatedAppSession{
		SessionID: sessionID,
		Version:   current.Version,
		Status:    status,
	}, nil
}

func (s *AppSessionService) lookupApp(ctx context.Context, client nitroliteClient, appID string) (*app.AppInfoV1, error) {
	if err := validateAppID(appID); err != nil {
		return nil, err
	}

	opts := &sdk.GetAppsOptions{AppID: &appID}
	apps, _, err := client.GetApps(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get apps: %w", err)
	}
	for _, appInfo := range apps {
		if appInfo.App.ID == appID {
			return &appInfo, nil
		}
	}
	return nil, notFoundf("app not found")
}

func (s *AppSessionService) lookupSession(ctx context.Context, client nitroliteClient, sessionID string) (*app.AppSessionInfoV1, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, invalidf("session_id is required")
	}

	opts := &sdk.GetAppSessionsOptions{AppSessionID: &sessionID}
	sessions, _, err := client.GetAppSessions(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get app sessions: %w", err)
	}
	for _, session := range sessions {
		if session.AppSessionID == sessionID {
			return &session, nil
		}
	}
	return nil, notFoundf("session not found")
}

type nitroliteClient interface {
	GetApps(ctx context.Context, opts *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error)
	GetAppSessions(ctx context.Context, opts *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error)
	GetAppDefinition(ctx context.Context, appSessionID string) (*app.AppDefinitionV1, error)
	CreateAppSession(ctx context.Context, definition app.AppDefinitionV1, sessionData string, quorumSigs []string, opts ...sdk.CreateAppSessionOptions) (string, string, string, error)
	SubmitAppSessionDeposit(ctx context.Context, appStateUpdate app.AppStateUpdateV1, quorumSigs []string, asset string, depositAmount decimal.Decimal) (string, error)
	SubmitAppState(ctx context.Context, appStateUpdate app.AppStateUpdateV1, quorumSigs []string) error
	RegisterApp(ctx context.Context, appID string, metadata string, creationApprovalNotRequired bool) error
}

func normalizeStatusFilter(raw string) (*string, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	switch trimmed {
	case "", "all":
		return nil, nil
	case "open", "closed":
		return &trimmed, nil
	default:
		return nil, invalidf("invalid status")
	}
}

func parseDecimalVersion(raw string) (uint64, error) {
	parsed, err := decimal.NewFromString(raw)
	if err != nil {
		return 0, invalidf("invalid version")
	}
	if !parsed.IsInteger() || parsed.IsNegative() {
		return 0, invalidf("invalid version")
	}
	return uint64(parsed.IntPart()), nil
}
