package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	appsigning "github.com/layer-3/nitrolite-store-example/internal/signing"
	"github.com/layer-3/nitrolite-store-example/internal/store"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/rpc"
	sdk "github.com/layer-3/nitrolite/sdk/go"
)

type StoreBootstrapResponse struct {
	StoreName        string              `json:"store_name"`
	AppID            string              `json:"app_id"`
	AppSigner        string              `json:"app_signer"`
	WalletAddress    string              `json:"wallet_address"`
	SelectedAsset    string              `json:"selected_asset"`
	DefaultAsset     string              `json:"default_asset"`
	SupportedAssets  []string            `json:"supported_assets"`
	AvailableBalance string              `json:"available_balance"`
	Catalog          []StoreCatalogItem  `json:"catalog"`
	Session          StoreShopperSession `json:"session"`
	Library          []StoreLibraryItem  `json:"library"`
}

type StoreShopperSession struct {
	Asset          string `json:"asset"`
	AppSessionID   string `json:"app_session_id,omitempty"`
	Status         string `json:"status"`
	Version        uint64 `json:"version"`
	UserAllocation string `json:"user_allocation"`
	AppAllocation  string `json:"app_allocation"`
	SessionData    string `json:"session_data,omitempty"`
}

type StoreLibraryItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Price       string `json:"price"`
	PurchasedAt string `json:"purchased_at"`
}

type StoreUpdateRequest struct {
	WalletAddress  string                `json:"wallet_address,omitempty"`
	Asset          string                `json:"asset"`
	AppStateUpdate *rpc.AppStateUpdateV1 `json:"app_state_update,omitempty"`
	UserSignature  string                `json:"user_signature,omitempty"`
}

type StoreInitRequest struct {
	WalletAddress string              `json:"wallet_address,omitempty"`
	Asset         string              `json:"asset"`
	Definition    rpc.AppDefinitionV1 `json:"definition"`
	SessionData   string              `json:"session_data,omitempty"`
	UserSignature string              `json:"user_signature"`
}

type StoreContentRequest struct {
	WalletAddress string
	Asset         string
}

type StoreUpdateResponse struct {
	Status       string                  `json:"status"`
	Intent       string                  `json:"intent"`
	Asset        string                  `json:"asset"`
	AppSessionID string                  `json:"app_session_id"`
	AppSignature string                  `json:"app_signature,omitempty"`
	Bootstrap    *StoreBootstrapResponse `json:"bootstrap,omitempty"`
}

type submitAppStateFunc func(ctx context.Context, wsURL string, req rpc.AppSessionsV1SubmitAppStateRequest) error
type createAppSessionFunc func(ctx context.Context, wsURL string, req rpc.AppSessionsV1CreateAppSessionRequest) (*rpc.AppSessionsV1CreateAppSessionResponse, error)

type WalletStoreService struct {
	provider            clientProvider
	store               *store.Store
	appSigner           appsigning.Signer
	storeName           string
	appID               string
	defaultAsset        string
	supportedAssets     []string
	catalog             []StoreCatalogItem
	wsURL               string
	now                 func() time.Time
	submitAppStateRPC   submitAppStateFunc
	createAppSessionRPC createAppSessionFunc
}

func NewWalletStoreService(provider clientProvider, appStore *store.Store, appSigner appsigning.Signer, storeName string, appID string, homeBlockchains map[string]uint64, wsURL string) *WalletStoreService {
	assets := supportedStoreAssets(homeBlockchains)
	defaultAsset := "yusd"
	if len(assets) > 0 {
		defaultAsset = assets[0]
		for _, asset := range assets {
			if asset == "yusd" {
				defaultAsset = asset
				break
			}
		}
	}

	return &WalletStoreService{
		provider:            provider,
		store:               appStore,
		appSigner:           appSigner,
		storeName:           strings.TrimSpace(storeName),
		appID:               strings.TrimSpace(appID),
		defaultAsset:        defaultAsset,
		supportedAssets:     assets,
		catalog:             seededCatalog(),
		wsURL:               strings.TrimSpace(wsURL),
		now:                 time.Now,
		submitAppStateRPC:   submitAppStateRPC,
		createAppSessionRPC: createAppSessionRPC,
	}
}

func (s *WalletStoreService) Bootstrap(ctx context.Context, walletAddress string, asset string) (*StoreBootstrapResponse, error) {
	asset, err := s.normalizeAsset(asset)
	if err != nil {
		return nil, err
	}

	catalog, err := s.catalogForAsset(asset)
	if err != nil {
		return nil, err
	}
	availableBalance, err := s.availableBalance(ctx, walletAddress, asset)
	if err != nil {
		return nil, err
	}
	session := StoreShopperSession{
		Asset:          asset,
		Status:         "missing",
		UserAllocation: "0",
		AppAllocation:  "0",
	}

	stored, err := s.store.GetWalletSession(ctx, walletAddress, asset)
	if err != nil && err != store.ErrNotFound {
		return nil, fmt.Errorf("failed to load wallet session: %w", err)
	}
	if stored != nil {
		current, lookupErr := s.lookupSession(ctx, stored.AppSessionID)
		if lookupErr == nil {
			summary, syncErr := s.sessionSummary(ctx, walletAddress, asset, *current)
			if syncErr != nil {
				return nil, syncErr
			}
			session = *summary
			if err := s.reconcilePendingPurchases(ctx, walletAddress, asset, *current); err != nil {
				return nil, err
			}
		} else {
			session = StoreShopperSession{
				Asset:          asset,
				AppSessionID:   stored.AppSessionID,
				Status:         "sync_failed",
				Version:        stored.Version,
				UserAllocation: stored.UserAllocation,
				AppAllocation:  stored.AppAllocation,
				SessionData:    stored.SessionData,
			}
		}
	}

	library, err := s.library(ctx, walletAddress, asset)
	if err != nil {
		return nil, err
	}

	return &StoreBootstrapResponse{
		StoreName:        s.storeName,
		AppID:            s.appID,
		AppSigner:        s.appSigner.Address(),
		WalletAddress:    walletAddress,
		SelectedAsset:    asset,
		DefaultAsset:     s.defaultAsset,
		SupportedAssets:  append([]string(nil), s.supportedAssets...),
		AvailableBalance: availableBalance,
		Catalog:          catalog,
		Session:          session,
		Library:          library,
	}, nil
}

func (s *WalletStoreService) Content(ctx context.Context, id string, req StoreContentRequest) (*StoreCatalogItem, error) {
	walletAddress := strings.TrimSpace(req.WalletAddress)
	if walletAddress == "" {
		return nil, invalidf("wallet_address is required")
	}

	asset, err := s.normalizeAsset(req.Asset)
	if err != nil {
		return nil, err
	}

	item := s.catalogItem(id)
	if item == nil {
		return nil, notFoundf("catalog item not found")
	}

	stored, err := s.store.GetWalletSession(ctx, walletAddress, asset)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, notFoundf("store session not found")
		}
		return nil, fmt.Errorf("failed to load wallet session: %w", err)
	}

	current, err := s.lookupSession(ctx, stored.AppSessionID)
	if err != nil {
		return nil, err
	}
	if err := s.validateSessionParticipants(current, walletAddress); err != nil {
		return nil, err
	}
	if err := s.reconcilePendingPurchases(ctx, walletAddress, asset, *current); err != nil {
		return nil, err
	}

	owned, err := s.store.HasWalletPurchase(ctx, walletAddress, id, asset)
	if err != nil {
		return nil, fmt.Errorf("failed to check purchase: %w", err)
	}
	if !owned {
		return nil, conflictf("item not purchased")
	}

	price := item.Prices[asset]
	copyItem := *item
	copyItem.Prices = map[string]string{asset: price}
	return &copyItem, nil
}

func (s *WalletStoreService) CreateSession(ctx context.Context, req StoreInitRequest) (*StoreBootstrapResponse, error) {
	asset, err := s.normalizeAsset(req.Asset)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.UserSignature) == "" {
		return nil, invalidf("user_signature is required")
	}
	if err := s.ensureApp(ctx); err != nil {
		return nil, err
	}

	definition, err := appDefinitionFromRPC(req.Definition)
	if err != nil {
		return nil, err
	}
	walletAddress, err := s.walletFromDefinition(definition, req.WalletAddress)
	if err != nil {
		return nil, err
	}
	if err := s.validateCreateDefinition(definition, walletAddress); err != nil {
		return nil, err
	}
	if err := verifyCreateSessionSignature(walletAddress, definition, req.SessionData, req.UserSignature); err != nil {
		return nil, err
	}

	appSig, err := signCreateAppSessionRequest(definition, req.SessionData, s.appSigner)
	if err != nil {
		return nil, err
	}

	createReq := rpc.AppSessionsV1CreateAppSessionRequest{
		Definition:  req.Definition,
		SessionData: req.SessionData,
		QuorumSigs:  []string{req.UserSignature, appSig},
	}
	resp, err := s.createAppSessionRPC(ctx, s.wsURL, createReq)
	if err != nil {
		return nil, upstreamf(err, "failed to create app session")
	}

	now := s.now().UTC()
	if err := s.store.UpsertWalletSession(ctx, store.WalletStoreSession{
		WalletAddress:  walletAddress,
		Asset:          asset,
		AppSessionID:   resp.AppSessionID,
		Status:         resp.Status,
		Version:        mustUint64(resp.Version),
		UserAllocation: "0",
		AppAllocation:  "0",
		SessionData:    req.SessionData,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist wallet session: %w", err)
	}

	return s.Bootstrap(ctx, walletAddress, asset)
}

func (s *WalletStoreService) SubmitUpdate(ctx context.Context, req StoreUpdateRequest) (*StoreUpdateResponse, error) {
	asset, err := s.normalizeAsset(req.Asset)
	if err != nil {
		return nil, err
	}
	if req.AppStateUpdate == nil {
		return nil, invalidf("app_state_update is required")
	}

	update, err := appStateUpdateFromRPC(*req.AppStateUpdate)
	if err != nil {
		return nil, err
	}

	switch update.Intent {
	case app.AppStateUpdateIntentDeposit:
		return s.submitDeposit(ctx, asset, req, update)
	case app.AppStateUpdateIntentWithdraw, app.AppStateUpdateIntentOperate:
		return s.submitAppStateUpdate(ctx, asset, req, update)
	default:
		return nil, invalidf("unsupported app update intent")
	}
}

func (s *WalletStoreService) submitDeposit(ctx context.Context, asset string, req StoreUpdateRequest, update app.AppStateUpdateV1) (*StoreUpdateResponse, error) {
	if req.AppStateUpdate == nil {
		return nil, invalidf("app_state_update is required")
	}
	if strings.TrimSpace(req.UserSignature) == "" {
		return nil, invalidf("user_signature is required")
	}

	current, walletAddress, err := s.currentWalletSessionByAppSessionID(ctx, update.AppSessionID, asset)
	if err != nil {
		return nil, err
	}
	if req.WalletAddress != "" && !strings.EqualFold(req.WalletAddress, walletAddress) {
		return nil, conflictf("wallet_address does not match app session owner")
	}
	if err := s.validateSessionParticipants(current, walletAddress); err != nil {
		return nil, err
	}
	if err := verifyAppStateSignature(walletAddress, update, req.UserSignature); err != nil {
		return nil, err
	}

	if err := s.validateDeposit(walletAddress, asset, current, update); err != nil {
		return nil, err
	}

	appSig, err := signAppStateUpdate(update, s.appSigner)
	if err != nil {
		return nil, err
	}

	return &StoreUpdateResponse{
		Status:       "signed",
		Intent:       string(StoreIntentUserDeposit),
		Asset:        asset,
		AppSessionID: update.AppSessionID,
		AppSignature: appSig,
	}, nil
}

func (s *WalletStoreService) submitAppStateUpdate(ctx context.Context, asset string, req StoreUpdateRequest, update app.AppStateUpdateV1) (*StoreUpdateResponse, error) {
	if req.AppStateUpdate == nil {
		return nil, invalidf("app_state_update is required")
	}
	if strings.TrimSpace(req.UserSignature) == "" {
		return nil, invalidf("user_signature is required")
	}

	current, walletAddress, err := s.currentWalletSessionByAppSessionID(ctx, update.AppSessionID, asset)
	if err != nil {
		return nil, err
	}
	if req.WalletAddress != "" && !strings.EqualFold(req.WalletAddress, walletAddress) {
		return nil, conflictf("wallet_address does not match app session owner")
	}
	if err := s.validateSessionParticipants(current, walletAddress); err != nil {
		return nil, err
	}
	if err := verifyAppStateSignature(walletAddress, update, req.UserSignature); err != nil {
		return nil, err
	}

	purchaseItemID := ""
	responseIntent := string(StoreIntentUserWithdraw)
	switch update.Intent {
	case app.AppStateUpdateIntentOperate:
		if err := s.reconcilePendingPurchases(ctx, walletAddress, asset, *current); err != nil {
			return nil, err
		}
		purchaseItemID, err = s.validatePurchase(ctx, walletAddress, asset, current, update)
		if err != nil {
			return nil, err
		}
		responseIntent = string(StoreIntentPurchase)
	case app.AppStateUpdateIntentWithdraw:
		if err := s.validateWithdraw(walletAddress, asset, current, update); err != nil {
			return nil, err
		}
	default:
		return nil, invalidf("unsupported app update intent")
	}

	appSig, err := signAppStateUpdate(update, s.appSigner)
	if err != nil {
		return nil, err
	}

	purchaseID := ""
	if purchaseItemID != "" {
		purchaseID = store.WalletPurchaseID(walletAddress, asset, purchaseItemID)
		now := s.now().UTC()
		if err := s.store.UpsertPendingWalletPurchase(ctx, store.WalletPurchase{
			ID:            purchaseID,
			WalletAddress: walletAddress,
			ItemID:        purchaseItemID,
			Asset:         asset,
			AppSessionID:  current.AppSessionID,
			Version:       update.Version,
			Status:        store.WalletPurchaseStatusPending,
			SessionData:   update.SessionData,
			CreatedAt:     now,
			UpdatedAt:     now,
			PurchasedAt:   now,
		}); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return nil, conflictCodef("duplicate_purchase", "item already purchased")
			}
			return nil, fmt.Errorf("failed to prepare purchase: %w", err)
		}
	}

	if err := s.submitAppStateRPC(ctx, s.wsURL, rpc.AppSessionsV1SubmitAppStateRequest{
		AppStateUpdate: *req.AppStateUpdate,
		QuorumSigs:     []string{req.UserSignature, appSig},
	}); err != nil {
		if purchaseID != "" {
			_ = s.store.MarkWalletPurchaseFailed(ctx, purchaseID, s.now().UTC())
		}
		return nil, upstreamf(err, "failed to submit app state")
	}

	if purchaseID != "" {
		if err := s.store.MarkWalletPurchaseSubmitted(ctx, purchaseID, s.now().UTC()); err != nil {
			return nil, fmt.Errorf("failed to mark purchase submitted: %w", err)
		}
	}

	bootstrap, err := s.Bootstrap(ctx, walletAddress, asset)
	if err != nil {
		return nil, err
	}
	return &StoreUpdateResponse{
		Status:       "submitted",
		Intent:       responseIntent,
		Asset:        asset,
		AppSessionID: update.AppSessionID,
		Bootstrap:    bootstrap,
	}, nil
}

func (s *WalletStoreService) catalogForAsset(asset string) ([]StoreCatalogItem, error) {
	out := make([]StoreCatalogItem, 0, len(s.catalog))
	for _, item := range s.catalog {
		price, ok := item.Prices[asset]
		if !ok {
			continue
		}
		copyItem := item
		copyItem.Prices = map[string]string{asset: price}
		copyItem.Content = ""
		out = append(out, copyItem)
	}
	return out, nil
}

func (s *WalletStoreService) normalizeAsset(raw string) (string, error) {
	asset := strings.ToLower(strings.TrimSpace(raw))
	if asset == "" {
		asset = s.defaultAsset
	}
	for _, supported := range s.supportedAssets {
		if asset == supported {
			return asset, nil
		}
	}
	return "", invalidf("unsupported asset")
}

func (s *WalletStoreService) availableBalance(ctx context.Context, walletAddress string, asset string) (string, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return "", err
	}
	balances, err := client.GetBalances(ctx, walletAddress)
	if err != nil {
		return "", upstreamf(err, "failed to get balances")
	}
	for _, entry := range balances {
		if strings.EqualFold(entry.Asset, asset) {
			return entry.Balance.String(), nil
		}
	}
	return "0", nil
}

func (s *WalletStoreService) lookupSession(ctx context.Context, sessionID string) (*app.AppSessionInfoV1, error) {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}
	opts := &sdk.GetAppSessionsOptions{AppSessionID: &sessionID}
	sessions, _, err := client.GetAppSessions(ctx, opts)
	if err != nil {
		return nil, upstreamf(err, "failed to get app sessions")
	}
	for _, session := range sessions {
		if session.AppSessionID == sessionID {
			return &session, nil
		}
	}
	return nil, notFoundf("session not found")
}

func (s *WalletStoreService) currentWalletSession(ctx context.Context, walletAddress string, asset string) (*app.AppSessionInfoV1, error) {
	stored, err := s.store.GetWalletSession(ctx, walletAddress, asset)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, notFoundf("store session not found")
		}
		return nil, fmt.Errorf("failed to load wallet session: %w", err)
	}
	return s.lookupSession(ctx, stored.AppSessionID)
}

func (s *WalletStoreService) currentWalletSessionByAppSessionID(ctx context.Context, appSessionID string, asset string) (*app.AppSessionInfoV1, string, error) {
	stored, err := s.store.GetWalletSessionByAppSessionID(ctx, appSessionID, asset)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, "", notFoundf("store session not found")
		}
		return nil, "", fmt.Errorf("failed to load wallet session: %w", err)
	}
	current, err := s.lookupSession(ctx, stored.AppSessionID)
	if err != nil {
		return nil, "", err
	}
	return current, stored.WalletAddress, nil
}

func (s *WalletStoreService) sessionSummary(ctx context.Context, walletAddress string, asset string, current app.AppSessionInfoV1) (*StoreShopperSession, error) {
	userAmount, appAmount := displayBalancesForAsset(current.Allocations, walletAddress, s.appSigner.Address(), asset)
	now := s.now().UTC()
	if err := s.store.UpsertWalletSession(ctx, store.WalletStoreSession{
		WalletAddress:  walletAddress,
		Asset:          asset,
		AppSessionID:   current.AppSessionID,
		Status:         openClosedStatus(current.IsClosed),
		Version:        current.Version,
		UserAllocation: userAmount.String(),
		AppAllocation:  appAmount.String(),
		SessionData:    current.SessionData,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		return nil, fmt.Errorf("failed to sync wallet session: %w", err)
	}
	return &StoreShopperSession{
		Asset:          asset,
		AppSessionID:   current.AppSessionID,
		Status:         openClosedStatus(current.IsClosed),
		Version:        current.Version,
		UserAllocation: userAmount.String(),
		AppAllocation:  appAmount.String(),
		SessionData:    current.SessionData,
	}, nil
}

func (s *WalletStoreService) library(ctx context.Context, walletAddress string, asset string) ([]StoreLibraryItem, error) {
	purchases, err := s.store.ListPurchasesByWallet(ctx, walletAddress, asset)
	if err != nil {
		return nil, fmt.Errorf("failed to list purchases: %w", err)
	}
	items := make([]StoreLibraryItem, 0, len(purchases))
	for _, purchase := range purchases {
		item := s.catalogItem(purchase.ItemID)
		if item == nil {
			continue
		}
		items = append(items, StoreLibraryItem{
			ID:          item.ID,
			Title:       item.Title,
			Description: item.Description,
			Type:        item.Type,
			Price:       item.Prices[asset],
			PurchasedAt: purchase.PurchasedAt.Format(time.RFC3339),
		})
	}
	return items, nil
}

func (s *WalletStoreService) reconcilePendingPurchases(ctx context.Context, walletAddress string, asset string, current app.AppSessionInfoV1) error {
	pending, err := s.store.ListPendingWalletPurchases(ctx, walletAddress, asset)
	if err != nil {
		return fmt.Errorf("failed to list pending purchases: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}

	for _, purchase := range pending {
		if purchase.AppSessionID != current.AppSessionID {
			continue
		}
		if current.Version < purchase.Version {
			continue
		}
		if strings.TrimSpace(current.SessionData) != strings.TrimSpace(purchase.SessionData) {
			continue
		}
		if err := s.store.MarkWalletPurchaseSubmitted(ctx, purchase.ID, s.now().UTC()); err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("failed to reconcile purchase: %w", err)
		}
	}
	return nil
}

func (s *WalletStoreService) walletFromDefinition(definition app.AppDefinitionV1, requestedWallet string) (string, error) {
	walletAddress := strings.TrimSpace(requestedWallet)
	for _, participant := range definition.Participants {
		if strings.EqualFold(participant.WalletAddress, s.appSigner.Address()) {
			continue
		}
		if walletAddress != "" && !strings.EqualFold(walletAddress, participant.WalletAddress) {
			return "", conflictf("wallet_address does not match app session participant")
		}
		walletAddress = strings.TrimSpace(participant.WalletAddress)
	}
	if walletAddress == "" {
		return "", invalidf("wallet participant is required")
	}
	return walletAddress, nil
}

func (s *WalletStoreService) validateCreateDefinition(definition app.AppDefinitionV1, walletAddress string) error {
	if strings.TrimSpace(definition.ApplicationID) != s.appID {
		return invalidf("application_id does not match configured app")
	}
	if definition.Quorum != 2 {
		return invalidf("create session quorum must be 2")
	}
	if len(definition.Participants) != 2 {
		return invalidf("create session requires exactly two participants")
	}

	seenWallet := false
	seenApp := false
	for _, participant := range definition.Participants {
		switch {
		case strings.EqualFold(participant.WalletAddress, walletAddress):
			seenWallet = participant.SignatureWeight == 1
		case strings.EqualFold(participant.WalletAddress, s.appSigner.Address()):
			seenApp = participant.SignatureWeight == 1
		default:
			return invalidf("unexpected app session participant")
		}
	}
	if !seenWallet || !seenApp {
		return invalidf("app session participants must be wallet and app signer")
	}
	return nil
}

func (s *WalletStoreService) validateSessionParticipants(current *app.AppSessionInfoV1, walletAddress string) error {
	if current == nil {
		return notFoundf("store session not found")
	}
	if current.IsClosed {
		return conflictf("store session is closed")
	}
	if current.AppDefinition.ApplicationID != s.appID {
		return conflictf("unexpected app session application")
	}
	if len(current.AppDefinition.Participants) != 2 {
		return conflictf("unexpected app session participant set")
	}
	if current.AppDefinition.Quorum != 2 {
		return conflictf("unexpected app session quorum")
	}

	seenWallet := false
	seenApp := false
	for _, participant := range current.AppDefinition.Participants {
		switch {
		case strings.EqualFold(participant.WalletAddress, walletAddress):
			seenWallet = participant.SignatureWeight == 1
		case strings.EqualFold(participant.WalletAddress, s.appSigner.Address()):
			seenApp = participant.SignatureWeight == 1
		}
	}
	if !seenWallet || !seenApp {
		return conflictf("app session is not owned by the connected wallet")
	}
	return nil
}

func (s *WalletStoreService) validatePurchase(ctx context.Context, walletAddress string, asset string, current *app.AppSessionInfoV1, update app.AppStateUpdateV1) (string, error) {
	if update.Intent != app.AppStateUpdateIntentOperate {
		return "", invalidf("purchase must use operate intent")
	}
	if update.AppSessionID != current.AppSessionID {
		return "", conflictf("app_session_id does not match active store session")
	}
	if update.Version != current.Version+1 {
		return "", conflictCodef("stale_version", "app session version is stale")
	}

	var sessionData StoreSessionData
	if err := json.Unmarshal([]byte(strings.TrimSpace(update.SessionData)), &sessionData); err != nil {
		return "", invalidf("invalid session_data")
	}
	if sessionData.Intent != StoreIntentPurchase {
		return "", invalidf("purchase session_data.intent must be purchase")
	}
	itemID, err := sessionDataItemID(sessionData.ItemID)
	if err != nil {
		return "", err
	}
	item := s.catalogItem(itemID)
	if item == nil {
		return "", notFoundf("catalog item not found")
	}
	expectedPrice := item.Prices[asset]
	expectedAmount, err := parsePositiveAmount(expectedPrice)
	if err != nil {
		return "", fmt.Errorf("invalid catalog price for %s: %w", item.ID, err)
	}
	sessionAmount, err := parsePositiveAmount(sessionData.ItemPrice)
	if err != nil {
		return "", err
	}
	if !expectedAmount.Equal(sessionAmount) {
		return "", conflictf("purchase price does not match catalog")
	}
	alreadyOwned, err := s.store.HasWalletPurchase(ctx, walletAddress, item.ID, asset)
	if err != nil {
		return "", fmt.Errorf("failed to check purchase: %w", err)
	}
	if alreadyOwned {
		return "", conflictCodef("duplicate_purchase", "item already purchased")
	}

	price := sessionAmount
	currentUser, currentApp, err := currentBalancesForAsset(current.Allocations, walletAddress, s.appSigner.Address(), asset)
	if err != nil {
		return "", err
	}
	nextUser, nextApp, err := strictBalancesForAsset(update.Allocations, walletAddress, s.appSigner.Address(), asset)
	if err != nil {
		return "", err
	}
	if !nextUser.Equal(currentUser.Sub(price)) {
		return "", conflictf("purchase user allocation delta is invalid")
	}
	if !nextApp.Equal(currentApp.Add(price)) {
		return "", conflictf("purchase app allocation delta is invalid")
	}
	if !nextUser.Add(nextApp).Equal(currentUser.Add(currentApp)) {
		return "", conflictf("purchase allocations must conserve total balance")
	}
	if nextUser.IsNegative() {
		return "", conflictCodef("insufficient_balance", "purchase would overdraw user allocation")
	}
	return item.ID, nil
}

func (s *WalletStoreService) validateWithdraw(walletAddress string, asset string, current *app.AppSessionInfoV1, update app.AppStateUpdateV1) error {
	if update.Intent != app.AppStateUpdateIntentWithdraw {
		return invalidf("withdraw must use withdraw intent")
	}
	if update.AppSessionID != current.AppSessionID {
		return conflictf("app_session_id does not match active store session")
	}
	if update.Version != current.Version+1 {
		return conflictCodef("stale_version", "app session version is stale")
	}

	var sessionData StoreSessionData
	if err := json.Unmarshal([]byte(strings.TrimSpace(update.SessionData)), &sessionData); err != nil {
		return invalidf("invalid session_data")
	}
	if sessionData.Intent != StoreIntentUserWithdraw {
		return invalidf("withdraw session_data.intent must be user_withdraw")
	}
	currentUser, currentApp, err := currentBalancesForAsset(current.Allocations, walletAddress, s.appSigner.Address(), asset)
	if err != nil {
		return err
	}
	nextUser, nextApp, err := strictBalancesForAsset(update.Allocations, walletAddress, s.appSigner.Address(), asset)
	if err != nil {
		return err
	}
	amount := currentUser.Sub(nextUser)
	if !amount.IsPositive() {
		return conflictf("withdraw amount must be positive")
	}
	if strings.TrimSpace(sessionData.Amount) != "" {
		sessionAmount, err := parsePositiveAmount(sessionData.Amount)
		if err != nil {
			return err
		}
		if !sessionAmount.Equal(amount) {
			return conflictf("withdraw session_data amount does not match allocation delta")
		}
	}
	if !nextUser.Equal(currentUser.Sub(amount)) {
		return conflictf("withdraw user allocation delta is invalid")
	}
	if !nextApp.Equal(currentApp) {
		return conflictf("withdraw cannot change app allocation")
	}
	if nextUser.IsNegative() {
		return conflictCodef("insufficient_balance", "withdraw would overdraw user allocation")
	}
	return nil
}

func (s *WalletStoreService) validateDeposit(walletAddress string, asset string, current *app.AppSessionInfoV1, update app.AppStateUpdateV1) error {
	if update.Intent != app.AppStateUpdateIntentDeposit {
		return invalidf("deposit must use deposit intent")
	}
	if update.AppSessionID != current.AppSessionID {
		return conflictf("app_session_id does not match active store session")
	}
	if update.Version != current.Version+1 {
		return conflictCodef("stale_version", "app session version is stale")
	}

	var sessionData StoreSessionData
	if err := json.Unmarshal([]byte(strings.TrimSpace(update.SessionData)), &sessionData); err != nil {
		return invalidf("invalid session_data")
	}
	if sessionData.Intent != StoreIntentUserDeposit {
		return invalidf("deposit session_data.intent must be user_deposit")
	}

	currentUser, currentApp, err := currentBalancesForAsset(current.Allocations, walletAddress, s.appSigner.Address(), asset)
	if err != nil {
		return err
	}
	nextUser, nextApp, err := strictBalancesForAsset(update.Allocations, walletAddress, s.appSigner.Address(), asset)
	if err != nil {
		return err
	}
	amount := nextUser.Sub(currentUser)
	if !amount.IsPositive() {
		return conflictf("deposit amount must be positive")
	}
	if strings.TrimSpace(sessionData.Amount) != "" {
		sessionAmount, err := parsePositiveAmount(sessionData.Amount)
		if err != nil {
			return err
		}
		if !sessionAmount.Equal(amount) {
			return conflictf("deposit session_data amount does not match allocation delta")
		}
	}
	if !nextUser.Equal(currentUser.Add(amount)) {
		return conflictf("deposit user allocation delta is invalid")
	}
	if !nextApp.Equal(currentApp) {
		return conflictf("deposit cannot change app allocation")
	}
	return nil
}

func (s *WalletStoreService) ensureApp(ctx context.Context) error {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return err
	}
	opts := &sdk.GetAppsOptions{AppID: &s.appID}
	apps, _, err := client.GetApps(ctx, opts)
	if err != nil {
		return upstreamf(err, "failed to get apps")
	}
	for _, info := range apps {
		if info.App.ID == s.appID {
			return nil
		}
	}
	if err := client.RegisterApp(ctx, s.appID, `{"product":"store"}`, true); err != nil {
		return upstreamf(err, "failed to register store app")
	}
	return nil
}

func (s *WalletStoreService) catalogItem(id string) *StoreCatalogItem {
	id = strings.TrimSpace(id)
	for i := range s.catalog {
		if s.catalog[i].ID == id {
			return &s.catalog[i]
		}
	}
	return nil
}

func mustUint64(raw string) uint64 {
	version, err := parseNumericVersion(raw)
	if err != nil {
		return 0
	}
	return version
}
