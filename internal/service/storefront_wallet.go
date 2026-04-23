package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appsigning "github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite-go-example/internal/store"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
	"github.com/layer-3/nitrolite/pkg/rpc"
	sdk "github.com/layer-3/nitrolite/sdk/go"
	"github.com/shopspring/decimal"
)

const (
	storeUpdateKindCreateSession = "create_session"
	storeUpdateKindDeposit       = "submit_deposit_state"
	storeUpdateKindAppState      = "submit_app_state"
)

type StoreBootstrapResponse struct {
	StoreName       string              `json:"store_name"`
	AppID           string              `json:"app_id"`
	AppSigner       string              `json:"app_signer"`
	WalletAddress   string              `json:"wallet_address"`
	SelectedAsset   string              `json:"selected_asset"`
	DefaultAsset    string              `json:"default_asset"`
	SupportedAssets []string            `json:"supported_assets"`
	AvailableBalance string             `json:"available_balance"`
	Catalog         []StoreCatalogItem  `json:"catalog"`
	Session         StoreShopperSession `json:"session"`
	Library         []StoreLibraryItem  `json:"library"`
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
	Asset         string                `json:"asset"`
	Kind          string                `json:"kind"`
	Definition    *rpc.AppDefinitionV1  `json:"definition,omitempty"`
	SessionData   string                `json:"session_data,omitempty"`
	AppStateUpdate *rpc.AppStateUpdateV1 `json:"app_state_update,omitempty"`
	UserSignature string                `json:"user_signature,omitempty"`
	UserState     *rpc.StateV1          `json:"user_state,omitempty"`
}

type submitDepositFunc func(ctx context.Context, wsURL string, req rpc.AppSessionsV1SubmitDepositStateRequest) (string, error)
type submitAppStateFunc func(ctx context.Context, wsURL string, req rpc.AppSessionsV1SubmitAppStateRequest) error
type createAppSessionFunc func(ctx context.Context, wsURL string, req rpc.AppSessionsV1CreateAppSessionRequest) (*rpc.AppSessionsV1CreateAppSessionResponse, error)

type WalletStoreService struct {
	provider         clientProvider
	store            *store.Store
	appSigner        appsigning.Signer
	storeName        string
	appID            string
	defaultAsset     string
	supportedAssets  []string
	catalog          []StoreCatalogItem
	wsURL            string
	now              func() time.Time
	submitDepositRPC submitDepositFunc
	submitAppStateRPC submitAppStateFunc
	createAppSessionRPC createAppSessionFunc
}

func NewWalletStoreService(provider clientProvider, appStore *store.Store, appSigner appsigning.Signer, storeName string, appID string, homeBlockchains map[string]uint64, wsURL string) *WalletStoreService {
	assets := sortedAssets(homeBlockchains)
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
		provider:         provider,
		store:            appStore,
		appSigner:        appSigner,
		storeName:        strings.TrimSpace(storeName),
		appID:            strings.TrimSpace(appID),
		defaultAsset:     defaultAsset,
		supportedAssets:  assets,
		catalog:          seededCatalog(),
		wsURL:            strings.TrimSpace(wsURL),
		now:              time.Now,
		submitDepositRPC:    submitDepositStateRPC,
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
	library, err := s.library(ctx, walletAddress, asset)
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
		}
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

func (s *WalletStoreService) Content(ctx context.Context, walletAddress string, asset string, id string) (*StoreCatalogItem, error) {
	asset, err := s.normalizeAsset(asset)
	if err != nil {
		return nil, err
	}
	item := s.catalogItem(id)
	if item == nil {
		return nil, notFoundf("catalog item not found")
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

func (s *WalletStoreService) SubmitUpdate(ctx context.Context, walletAddress string, req StoreUpdateRequest) (*StoreBootstrapResponse, error) {
	asset, err := s.normalizeAsset(req.Asset)
	if err != nil {
		return nil, err
	}

	switch strings.TrimSpace(req.Kind) {
	case storeUpdateKindCreateSession:
		return s.createSession(ctx, walletAddress, asset, req)
	case storeUpdateKindDeposit:
		return s.submitDeposit(ctx, walletAddress, asset, req)
	case storeUpdateKindAppState:
		return s.submitAppStateUpdate(ctx, walletAddress, asset, req)
	default:
		return nil, invalidf("unsupported update kind")
	}
}

func (s *WalletStoreService) createSession(ctx context.Context, walletAddress string, asset string, req StoreUpdateRequest) (*StoreBootstrapResponse, error) {
	if req.Definition == nil {
		return nil, invalidf("definition is required")
	}
	if strings.TrimSpace(req.UserSignature) == "" {
		return nil, invalidf("user_signature is required")
	}
	if err := s.ensureApp(ctx); err != nil {
		return nil, err
	}

	definition, err := appDefinitionFromRPC(*req.Definition)
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
		Definition:   *req.Definition,
		SessionData:  req.SessionData,
		QuorumSigs:   []string{req.UserSignature, appSig},
	}
	resp, err := s.createAppSessionRPC(ctx, s.wsURL, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create app session: %w", err)
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

func (s *WalletStoreService) submitDeposit(ctx context.Context, walletAddress string, asset string, req StoreUpdateRequest) (*StoreBootstrapResponse, error) {
	if req.AppStateUpdate == nil {
		return nil, invalidf("app_state_update is required")
	}
	if req.UserState == nil {
		return nil, invalidf("user_state is required")
	}
	if strings.TrimSpace(req.UserSignature) == "" {
		return nil, invalidf("user_signature is required")
	}

	update, err := appStateUpdateFromRPC(*req.AppStateUpdate)
	if err != nil {
		return nil, err
	}
	userState, err := stateFromRPC(*req.UserState)
	if err != nil {
		return nil, err
	}

	current, err := s.currentWalletSession(ctx, walletAddress, asset)
	if err != nil {
		return nil, err
	}
	if err := s.validateSessionParticipants(current, walletAddress); err != nil {
		return nil, err
	}
	if err := verifyAppStateSignature(walletAddress, update, req.UserSignature); err != nil {
		return nil, err
	}

	depositAmount, err := s.validateDeposit(ctx, walletAddress, asset, current, update, userState)
	if err != nil {
		return nil, err
	}

	appSig, err := signAppStateUpdate(update, s.appSigner)
	if err != nil {
		return nil, err
	}

	_, err = s.submitDepositRPC(ctx, s.wsURL, rpc.AppSessionsV1SubmitDepositStateRequest{
		AppStateUpdate: *req.AppStateUpdate,
		QuorumSigs:     []string{req.UserSignature, appSig},
		UserState:      *req.UserState,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to submit deposit state: %w", err)
	}

	_ = depositAmount
	return s.Bootstrap(ctx, walletAddress, asset)
}

func (s *WalletStoreService) submitAppStateUpdate(ctx context.Context, walletAddress string, asset string, req StoreUpdateRequest) (*StoreBootstrapResponse, error) {
	if req.AppStateUpdate == nil {
		return nil, invalidf("app_state_update is required")
	}
	if strings.TrimSpace(req.UserSignature) == "" {
		return nil, invalidf("user_signature is required")
	}

	update, err := appStateUpdateFromRPC(*req.AppStateUpdate)
	if err != nil {
		return nil, err
	}

	current, err := s.currentWalletSession(ctx, walletAddress, asset)
	if err != nil {
		return nil, err
	}
	if err := s.validateSessionParticipants(current, walletAddress); err != nil {
		return nil, err
	}
	if err := verifyAppStateSignature(walletAddress, update, req.UserSignature); err != nil {
		return nil, err
	}

	purchaseItemID := ""
	switch update.Intent {
	case app.AppStateUpdateIntentOperate:
		purchaseItemID, err = s.validatePurchase(ctx, walletAddress, asset, current, update)
		if err != nil {
			return nil, err
		}
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
	if err := s.submitAppStateRPC(ctx, s.wsURL, rpc.AppSessionsV1SubmitAppStateRequest{
		AppStateUpdate: *req.AppStateUpdate,
		QuorumSigs:     []string{req.UserSignature, appSig},
	}); err != nil {
		return nil, fmt.Errorf("failed to submit app state: %w", err)
	}

	if purchaseItemID != "" {
		if err := s.store.RecordWalletPurchase(ctx, store.WalletPurchase{
			ID:            store.WalletPurchaseID(walletAddress, asset, purchaseItemID),
			WalletAddress: walletAddress,
			ItemID:        purchaseItemID,
			Asset:         asset,
			AppSessionID:  current.AppSessionID,
			Version:       update.Version,
			PurchasedAt:   s.now().UTC(),
		}); err != nil {
			return nil, fmt.Errorf("failed to record purchase: %w", err)
		}
	}

	return s.Bootstrap(ctx, walletAddress, asset)
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
		return "", fmt.Errorf("failed to get balances: %w", err)
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
		return nil, fmt.Errorf("failed to get app sessions: %w", err)
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

func (s *WalletStoreService) sessionSummary(ctx context.Context, walletAddress string, asset string, current app.AppSessionInfoV1) (*StoreShopperSession, error) {
	userAmount, appAmount := balancesForAsset(current.Allocations, walletAddress, s.appSigner.Address(), asset)
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
			seenWallet = participant.SignatureWeight > 0
		case strings.EqualFold(participant.WalletAddress, s.appSigner.Address()):
			seenApp = participant.SignatureWeight > 0
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

	seenWallet := false
	seenApp := false
	for _, participant := range current.AppDefinition.Participants {
		switch {
		case strings.EqualFold(participant.WalletAddress, walletAddress):
			seenWallet = true
		case strings.EqualFold(participant.WalletAddress, s.appSigner.Address()):
			seenApp = true
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
		return "", conflictf("app session version is stale")
	}

	var sessionData StoreSessionData
	if err := json.Unmarshal([]byte(strings.TrimSpace(update.SessionData)), &sessionData); err != nil {
		return "", invalidf("invalid session_data")
	}
	if sessionData.Action != ActionPurchase {
		return "", invalidf("purchase session_data.action must be purchase")
	}
	item := s.catalogItem(sessionData.ItemID)
	if item == nil {
		return "", notFoundf("catalog item not found")
	}
	expectedPrice := item.Prices[asset]
	expectedAmount, err := parsePositiveAmount(expectedPrice)
	if err != nil {
		return "", fmt.Errorf("invalid catalog price for %s: %w", item.ID, err)
	}
	sessionAmount, err := parsePositiveAmount(sessionData.Price)
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
		return "", conflictf("item already purchased")
	}

	price := sessionAmount
	currentUser, currentApp := balancesForAsset(current.Allocations, walletAddress, s.appSigner.Address(), asset)
	nextUser, nextApp := balancesForAsset(update.Allocations, walletAddress, s.appSigner.Address(), asset)
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
		return "", conflictf("purchase would overdraw user allocation")
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
		return conflictf("app session version is stale")
	}

	var sessionData StoreSessionData
	if err := json.Unmarshal([]byte(strings.TrimSpace(update.SessionData)), &sessionData); err != nil {
		return invalidf("invalid session_data")
	}
	if sessionData.Action != ActionUserWithdraw {
		return invalidf("withdraw session_data.action must be user_withdraw")
	}

	amount, err := parsePositiveAmount(sessionData.Amount)
	if err != nil {
		return err
	}
	currentUser, currentApp := balancesForAsset(current.Allocations, walletAddress, s.appSigner.Address(), asset)
	nextUser, nextApp := balancesForAsset(update.Allocations, walletAddress, s.appSigner.Address(), asset)
	if !nextUser.Equal(currentUser.Sub(amount)) {
		return conflictf("withdraw user allocation delta is invalid")
	}
	if !nextApp.Equal(currentApp) {
		return conflictf("withdraw cannot change app allocation")
	}
	if nextUser.IsNegative() {
		return conflictf("withdraw would overdraw user allocation")
	}
	return nil
}

func (s *WalletStoreService) validateDeposit(ctx context.Context, walletAddress string, asset string, current *app.AppSessionInfoV1, update app.AppStateUpdateV1, userState core.State) (decimal.Decimal, error) {
	if update.Intent != app.AppStateUpdateIntentDeposit {
		return decimal.Zero, invalidf("deposit must use deposit intent")
	}
	if update.AppSessionID != current.AppSessionID {
		return decimal.Zero, conflictf("app_session_id does not match active store session")
	}
	if update.Version != current.Version+1 {
		return decimal.Zero, conflictf("app session version is stale")
	}

	var sessionData StoreSessionData
	if err := json.Unmarshal([]byte(strings.TrimSpace(update.SessionData)), &sessionData); err != nil {
		return decimal.Zero, invalidf("invalid session_data")
	}
	if sessionData.Action != ActionDeposit {
		return decimal.Zero, invalidf("deposit session_data.action must be deposit")
	}

	amount, err := parsePositiveAmount(sessionData.Amount)
	if err != nil {
		return decimal.Zero, err
	}

	currentUser, currentApp := balancesForAsset(current.Allocations, walletAddress, s.appSigner.Address(), asset)
	nextUser, nextApp := balancesForAsset(update.Allocations, walletAddress, s.appSigner.Address(), asset)
	if !nextUser.Equal(currentUser.Add(amount)) {
		return decimal.Zero, conflictf("deposit user allocation delta is invalid")
	}
	if !nextApp.Equal(currentApp) {
		return decimal.Zero, conflictf("deposit cannot change app allocation")
	}

	assetStore, err := s.assetStore(ctx)
	if err != nil {
		return decimal.Zero, err
	}
	client, _, err := activeClient(s.provider)
	if err != nil {
		return decimal.Zero, err
	}
	currentState, err := client.GetLatestState(ctx, walletAddress, asset, false)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to get latest state: %w", err)
	}
	advancer := core.NewStateAdvancerV1(assetStore)
	if err := advancer.ValidateAdvancement(*currentState, userState); err != nil {
		return decimal.Zero, conflictf("user_state is not a valid next state: %v", err)
	}
	if userState.Transition.Type != core.TransitionTypeCommit {
		return decimal.Zero, conflictf("deposit user_state transition must be commit")
	}
	if !strings.EqualFold(userState.Transition.AccountID, update.AppSessionID) {
		return decimal.Zero, conflictf("deposit user_state account_id must equal app_session_id")
	}
	if !userState.Transition.Amount.Equal(amount) {
		return decimal.Zero, conflictf("deposit user_state amount must match session_data amount")
	}
	if !strings.EqualFold(userState.Asset, asset) {
		return decimal.Zero, conflictf("deposit user_state asset mismatch")
	}
	if !strings.EqualFold(userState.UserWallet, walletAddress) {
		return decimal.Zero, conflictf("deposit user_state wallet mismatch")
	}
	if userState.UserSig == nil || strings.TrimSpace(*userState.UserSig) == "" {
		return decimal.Zero, conflictf("deposit user_state is missing user_sig")
	}
	if err := verifyChannelStateSignature(walletAddress, userState, assetStore); err != nil {
		return decimal.Zero, err
	}
	return amount, nil
}

func (s *WalletStoreService) ensureApp(ctx context.Context) error {
	client, _, err := activeClient(s.provider)
	if err != nil {
		return err
	}
	opts := &sdk.GetAppsOptions{AppID: &s.appID}
	apps, _, err := client.GetApps(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to get apps: %w", err)
	}
	for _, info := range apps {
		if info.App.ID == s.appID {
			return nil
		}
	}
	if err := client.RegisterApp(ctx, s.appID, `{"product":"store"}`, true); err != nil {
		return fmt.Errorf("failed to register store app: %w", err)
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
