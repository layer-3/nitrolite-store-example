package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	appsigning "github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite-go-example/internal/store"
	"github.com/layer-3/nitrolite/pkg/app"
	sdk "github.com/layer-3/nitrolite/sdk/go"
	"github.com/shopspring/decimal"
)

const (
	storeActionDeposit      = "deposit"
	storeActionPurchase     = "purchase"
	storeActionUserWithdraw = "user_withdraw"
	storeActionAppWithdraw  = "app_withdraw"
)

type StoreCatalogItem struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Type        string            `json:"type"`
	Prices      map[string]string `json:"prices"`
	Content     string            `json:"content,omitempty"`
}

type StoreAction string

const (
	ActionDeposit      StoreAction = "deposit"
	ActionPurchase     StoreAction = "purchase"
	ActionUserWithdraw StoreAction = "user_withdraw"
	ActionAppWithdraw  StoreAction = "app_withdraw"
)

type StoreSessionData struct {
	Action StoreAction `json:"action"`
	Amount string      `json:"amount,omitempty"`
	ItemID string      `json:"item_id,omitempty"`
	Price  string      `json:"price,omitempty"`
}

type StoreConfigResponse struct {
	StoreName       string             `json:"store_name"`
	AppID           string             `json:"app_id"`
	AppSigner       string             `json:"app_signer"`
	UserSigner      string             `json:"user_signer"`
	DefaultAsset    string             `json:"default_asset"`
	SupportedAssets []string           `json:"supported_assets"`
	Catalog         []StoreCatalogItem `json:"catalog"`
}

type StoreSessionSummary struct {
	BrowserSessionID string           `json:"-"`
	Asset            string           `json:"asset"`
	AppSessionID     string           `json:"app_session_id"`
	Status           string           `json:"status"`
	Version          uint64           `json:"version"`
	UserAllocation   string           `json:"user_allocation"`
	AppAllocation    string           `json:"app_allocation"`
	SessionData      string           `json:"session_data"`
	AvailableBalance string           `json:"available_balance"`
	Purchases        []store.Purchase `json:"purchases"`
}

type StoreStateRequest struct {
	SessionID   string `json:"session_id"`
	Asset       string `json:"asset"`
	SessionData string `json:"session_data"`
}

type StorefrontService struct {
	provider        clientProvider
	store           *store.Store
	userSigner      appsigning.Signer
	appSigner       appsigning.Signer
	storeName       string
	appID           string
	supportedAssets []string
	defaultAsset    string
	catalog         []StoreCatalogItem
	now             func() time.Time
}

func NewStorefrontService(provider clientProvider, appStore *store.Store, userSigner appsigning.Signer, appSigner appsigning.Signer, storeName string, appID string, homeBlockchains map[string]uint64) *StorefrontService {
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

	return &StorefrontService{
		provider:        provider,
		store:           appStore,
		userSigner:      userSigner,
		appSigner:       appSigner,
		storeName:       strings.TrimSpace(storeName),
		appID:           strings.TrimSpace(appID),
		supportedAssets: assets,
		defaultAsset:    defaultAsset,
		catalog:         seededCatalog(),
		now:             time.Now,
	}
}

func (s *StorefrontService) Config() StoreConfigResponse {
	return StoreConfigResponse{
		StoreName:       s.storeName,
		AppID:           s.appID,
		AppSigner:       s.appSigner.Address(),
		UserSigner:      s.userSigner.Address(),
		DefaultAsset:    s.defaultAsset,
		SupportedAssets: append([]string(nil), s.supportedAssets...),
		Catalog:         cloneCatalogForAPI(s.catalog),
	}
}

func (s *StorefrontService) Catalog(asset string) ([]StoreCatalogItem, error) {
	asset, err := s.normalizeAsset(asset)
	if err != nil {
		return nil, err
	}

	out := make([]StoreCatalogItem, 0, len(s.catalog))
	for _, item := range s.catalog {
		if _, ok := item.Prices[asset]; ok {
			copyItem := item
			copyItem.Prices = map[string]string{asset: item.Prices[asset]}
			copyItem.Content = ""
			out = append(out, copyItem)
		}
	}
	return out, nil
}

func (s *StorefrontService) CatalogItem(asset string, id string) (*StoreCatalogItem, error) {
	asset, err := s.normalizeAsset(asset)
	if err != nil {
		return nil, err
	}

	item := s.catalogItem(id)
	if item == nil {
		return nil, notFoundf("catalog item not found")
	}
	price, ok := item.Prices[asset]
	if !ok {
		return nil, invalidf("asset not supported for item")
	}
	copyItem := *item
	copyItem.Prices = map[string]string{asset: price}
	copyItem.Content = ""
	return &copyItem, nil
}

func (s *StorefrontService) Content(ctx context.Context, browserSessionID string, asset string, id string) (*StoreCatalogItem, error) {
	asset, err := s.normalizeAsset(asset)
	if err != nil {
		return nil, err
	}
	item := s.catalogItem(id)
	if item == nil {
		return nil, notFoundf("catalog item not found")
	}
	owned, err := s.store.HasPurchase(ctx, browserSessionID, id, asset)
	if err != nil {
		return nil, fmt.Errorf("failed to check purchase: %w", err)
	}
	if !owned {
		return nil, conflictf("item not purchased")
	}
	copyItem := *item
	copyItem.Prices = map[string]string{asset: item.Prices[asset]}
	return &copyItem, nil
}

func (s *StorefrontService) GetSession(ctx context.Context, browserSessionID string, asset string) (*StoreSessionSummary, error) {
	asset, err := s.normalizeAsset(asset)
	if err != nil {
		return nil, err
	}

	stored, err := s.store.GetStoreSession(ctx, browserSessionID, asset)
	if err != nil {
		if err == store.ErrNotFound {
			balance, balErr := s.availableBalance(ctx, asset)
			if balErr != nil {
				return nil, balErr
			}
			return &StoreSessionSummary{
				BrowserSessionID: browserSessionID,
				Asset:            asset,
				Status:           "missing",
				AvailableBalance: balance,
			}, nil
		}
		return nil, fmt.Errorf("failed to load store session: %w", err)
	}

	current, err := s.lookupSession(ctx, stored.AppSessionID)
	if err != nil {
		return nil, err
	}
	summary, err := s.summaryFromAppSession(ctx, browserSessionID, asset, *current)
	if err != nil {
		return nil, err
	}
	return summary, nil
}

func (s *StorefrontService) CreateSession(ctx context.Context, browserSessionID string, asset string) (*StoreSessionSummary, error) {
	asset, err := s.normalizeAsset(asset)
	if err != nil {
		return nil, err
	}

	existing, err := s.store.GetStoreSession(ctx, browserSessionID, asset)
	if err == nil && existing != nil {
		return s.GetSession(ctx, browserSessionID, asset)
	}
	if err != nil && err != store.ErrNotFound {
		return nil, fmt.Errorf("failed to check store session: %w", err)
	}

	if err := s.ensureApp(ctx); err != nil {
		return nil, err
	}

	definition := app.AppDefinitionV1{
		ApplicationID: s.appID,
		Participants: []app.AppParticipantV1{
			{WalletAddress: s.userSigner.Address(), SignatureWeight: 1},
			{WalletAddress: s.appSigner.Address(), SignatureWeight: 1},
		},
		Quorum: 2,
		Nonce:  uint64(s.now().UTC().UnixNano()),
	}
	sessionData := mustJSONString(map[string]string{"action": "deposit", "amount": "0", "asset": asset})

	userSig, err := signCreateAppSessionRequest(definition, sessionData, s.userSigner)
	if err != nil {
		return nil, err
	}
	appSig, err := signCreateAppSessionRequest(definition, sessionData, s.appSigner)
	if err != nil {
		return nil, err
	}

	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}
	sessionID, versionRaw, status, err := client.CreateAppSession(ctx, definition, sessionData, []string{userSig, appSig})
	if err != nil {
		return nil, fmt.Errorf("failed to create app session: %w", err)
	}
	version, err := parseDecimalVersion(versionRaw)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	record := store.StoreSession{
		BrowserSessionID: browserSessionID,
		Asset:            asset,
		AppSessionID:     sessionID,
		Status:           status,
		Version:          version,
		UserAllocation:   "0",
		AppAllocation:    "0",
		SessionData:      sessionData,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.store.UpsertStoreSession(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to persist store session: %w", err)
	}

	return s.GetSession(ctx, browserSessionID, asset)
}

func (s *StorefrontService) SubmitState(ctx context.Context, browserSessionID string, req StoreStateRequest, allowAppWithdraw bool) (*StoreSessionSummary, error) {
	asset, err := s.normalizeAsset(req.Asset)
	if err != nil {
		return nil, err
	}
	stored, err := s.store.GetStoreSession(ctx, browserSessionID, asset)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, notFoundf("store session not found")
		}
		return nil, fmt.Errorf("failed to load store session: %w", err)
	}
	if strings.TrimSpace(req.SessionID) != "" && !strings.EqualFold(strings.TrimSpace(req.SessionID), stored.AppSessionID) {
		return nil, conflictf("session_id does not match active store session")
	}
	var sessionData StoreSessionData
	if err := json.Unmarshal([]byte(strings.TrimSpace(req.SessionData)), &sessionData); err != nil {
		return nil, invalidf("invalid session_data")
	}

	current, err := s.lookupSession(ctx, stored.AppSessionID)
	if err != nil {
		return nil, err
	}

	switch sessionData.Action {
	case ActionDeposit:
		return s.handleDeposit(ctx, browserSessionID, asset, *current, sessionData)
	case ActionPurchase:
		return s.handlePurchase(ctx, browserSessionID, asset, *current, sessionData)
	case ActionUserWithdraw:
		return s.handleWithdraw(ctx, browserSessionID, asset, *current, sessionData, false)
	case ActionAppWithdraw:
		if !allowAppWithdraw {
			return nil, conflictf("app_withdraw is only available from hidden developer controls")
		}
		return s.handleWithdraw(ctx, browserSessionID, asset, *current, sessionData, true)
	default:
		return nil, invalidf("invalid action")
	}
}

func (s *StorefrontService) handleDeposit(ctx context.Context, browserSessionID string, asset string, current app.AppSessionInfoV1, sessionData StoreSessionData) (*StoreSessionSummary, error) {
	amount, err := parsePositiveAmount(sessionData.Amount)
	if err != nil {
		return nil, err
	}
	update, err := buildDepositUpdate(current, s.userSigner.Address(), asset, amount)
	if err != nil {
		return nil, err
	}
	update.SessionData = mustJSONString(StoreSessionData{
		Action: ActionDeposit,
		Amount: amount.StringFixedBank(2),
	})

	sigs, err := s.signQuorumUpdate(update)
	if err != nil {
		return nil, err
	}
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}
	if _, err := client.SubmitAppSessionDeposit(ctx, update, sigs, asset, amount); err != nil {
		return nil, fmt.Errorf("failed to submit deposit state: %w", err)
	}
	return s.persistAndSummarize(ctx, browserSessionID, asset, update)
}

func (s *StorefrontService) handlePurchase(ctx context.Context, browserSessionID string, asset string, current app.AppSessionInfoV1, sessionData StoreSessionData) (*StoreSessionSummary, error) {
	item := s.catalogItem(sessionData.ItemID)
	if item == nil {
		return nil, notFoundf("catalog item not found")
	}
	priceRaw, ok := item.Prices[asset]
	if !ok {
		return nil, invalidf("asset not supported for item")
	}
	price, err := decimal.NewFromString(priceRaw)
	if err != nil {
		return nil, invalidf("invalid catalog price")
	}
	if strings.TrimSpace(sessionData.Price) != "" && sessionData.Price != price.StringFixedBank(2) {
		return nil, conflictf("session_data price does not match catalog price")
	}
	owned, err := s.store.HasPurchase(ctx, browserSessionID, item.ID, asset)
	if err != nil {
		return nil, fmt.Errorf("failed to check duplicate purchase: %w", err)
	}
	if owned {
		return nil, conflictf("item already purchased")
	}

	userAmount, appAmount := balancesForAsset(current.Allocations, s.userSigner.Address(), s.appSigner.Address(), asset)
	if userAmount.LessThan(price) {
		return nil, conflictf("insufficient store balance")
	}

	update := app.AppStateUpdateV1{
		AppSessionID: current.AppSessionID,
		Intent:       app.AppStateUpdateIntentOperate,
		Version:      current.Version + 1,
		Allocations: []app.AppAllocationV1{
			{Participant: s.appSigner.Address(), Asset: asset, Amount: appAmount.Add(price)},
			{Participant: s.userSigner.Address(), Asset: asset, Amount: userAmount.Sub(price)},
		},
		SessionData: mustJSONString(StoreSessionData{
			Action: ActionPurchase,
			ItemID: item.ID,
			Price:  price.StringFixedBank(2),
		}),
	}
	sortAppAllocations(update.Allocations)

	sigs, err := s.signQuorumUpdate(update)
	if err != nil {
		return nil, err
	}
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}
	if err := client.SubmitAppState(ctx, update, sigs); err != nil {
		return nil, fmt.Errorf("failed to submit purchase state: %w", err)
	}
	if err := s.store.RecordPurchase(ctx, store.Purchase{
		ID:               fmt.Sprintf("%s:%s:%s", browserSessionID, asset, item.ID),
		BrowserSessionID: browserSessionID,
		ItemID:           item.ID,
		Asset:            asset,
		AppSessionID:     current.AppSessionID,
		Version:          update.Version,
		PurchasedAt:      s.now().UTC(),
	}); err != nil {
		if err == store.ErrConflict {
			return nil, conflictf("item already purchased")
		}
		return nil, fmt.Errorf("failed to record purchase: %w", err)
	}
	return s.persistAndSummarize(ctx, browserSessionID, asset, update)
}

func (s *StorefrontService) handleWithdraw(ctx context.Context, browserSessionID string, asset string, current app.AppSessionInfoV1, sessionData StoreSessionData, appWithdraw bool) (*StoreSessionSummary, error) {
	amount, err := parsePositiveAmount(sessionData.Amount)
	if err != nil {
		return nil, err
	}
	userAmount, appAmount := balancesForAsset(current.Allocations, s.userSigner.Address(), s.appSigner.Address(), asset)

	intent := ActionUserWithdraw
	allocations := []app.AppAllocationV1{
		{Participant: s.appSigner.Address(), Asset: asset, Amount: appAmount},
		{Participant: s.userSigner.Address(), Asset: asset, Amount: userAmount},
	}
	if appWithdraw {
		intent = ActionAppWithdraw
		if appAmount.LessThan(amount) {
			return nil, conflictf("app allocation is too low")
		}
		allocations[0].Amount = appAmount.Sub(amount)
	} else {
		if userAmount.LessThan(amount) {
			return nil, conflictf("user allocation is too low")
		}
		allocations[1].Amount = userAmount.Sub(amount)
	}

	update := app.AppStateUpdateV1{
		AppSessionID: current.AppSessionID,
		Intent:       app.AppStateUpdateIntentWithdraw,
		Version:      current.Version + 1,
		Allocations:  allocations,
		SessionData: mustJSONString(StoreSessionData{
			Action: intent,
			Amount: amount.StringFixedBank(2),
		}),
	}
	sortAppAllocations(update.Allocations)

	sigs, err := s.signQuorumUpdate(update)
	if err != nil {
		return nil, err
	}
	client, _, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}
	if err := client.SubmitAppState(ctx, update, sigs); err != nil {
		return nil, fmt.Errorf("failed to submit withdraw state: %w", err)
	}
	return s.persistAndSummarize(ctx, browserSessionID, asset, update)
}

func (s *StorefrontService) persistAndSummarize(ctx context.Context, browserSessionID string, asset string, update app.AppStateUpdateV1) (*StoreSessionSummary, error) {
	userAmount, appAmount := balancesForAsset(update.Allocations, s.userSigner.Address(), s.appSigner.Address(), asset)
	now := s.now().UTC()
	record := store.StoreSession{
		BrowserSessionID: browserSessionID,
		Asset:            asset,
		AppSessionID:     update.AppSessionID,
		Status:           "open",
		Version:          update.Version,
		UserAllocation:   userAmount.String(),
		AppAllocation:    appAmount.String(),
		SessionData:      update.SessionData,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.store.UpsertStoreSession(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to update store session: %w", err)
	}
	return s.GetSession(ctx, browserSessionID, asset)
}

func (s *StorefrontService) summaryFromAppSession(ctx context.Context, browserSessionID string, asset string, current app.AppSessionInfoV1) (*StoreSessionSummary, error) {
	userAmount, appAmount := balancesForAsset(current.Allocations, s.userSigner.Address(), s.appSigner.Address(), asset)
	now := s.now().UTC()
	record := store.StoreSession{
		BrowserSessionID: browserSessionID,
		Asset:            asset,
		AppSessionID:     current.AppSessionID,
		Status:           openClosedStatus(current.IsClosed),
		Version:          current.Version,
		UserAllocation:   userAmount.String(),
		AppAllocation:    appAmount.String(),
		SessionData:      current.SessionData,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.store.UpsertStoreSession(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to sync store session: %w", err)
	}
	availableBalance, err := s.availableBalance(ctx, asset)
	if err != nil {
		return nil, err
	}
	purchases, err := s.store.ListPurchasesByBrowser(ctx, browserSessionID, asset)
	if err != nil {
		return nil, fmt.Errorf("failed to list purchases: %w", err)
	}
	return &StoreSessionSummary{
		BrowserSessionID: browserSessionID,
		Asset:            asset,
		AppSessionID:     current.AppSessionID,
		Status:           openClosedStatus(current.IsClosed),
		Version:          current.Version,
		UserAllocation:   userAmount.String(),
		AppAllocation:    appAmount.String(),
		SessionData:      current.SessionData,
		AvailableBalance: availableBalance,
		Purchases:        purchases,
	}, nil
}

func (s *StorefrontService) availableBalance(ctx context.Context, asset string) (string, error) {
	client, signerAddress, err := activeClient(s.provider)
	if err != nil {
		return "", err
	}
	balances, err := client.GetBalances(ctx, signerAddress)
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

func (s *StorefrontService) lookupSession(ctx context.Context, sessionID string) (*app.AppSessionInfoV1, error) {
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

func (s *StorefrontService) ensureApp(ctx context.Context) error {
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

func (s *StorefrontService) signQuorumUpdate(update app.AppStateUpdateV1) ([]string, error) {
	userSig, err := signAppStateUpdate(update, s.userSigner)
	if err != nil {
		return nil, err
	}
	appSig, err := signAppStateUpdate(update, s.appSigner)
	if err != nil {
		return nil, err
	}
	return []string{userSig, appSig}, nil
}

func (s *StorefrontService) normalizeAsset(raw string) (string, error) {
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

func (s *StorefrontService) catalogItem(id string) *StoreCatalogItem {
	id = strings.TrimSpace(id)
	for i := range s.catalog {
		if s.catalog[i].ID == id {
			return &s.catalog[i]
		}
	}
	return nil
}

func cloneCatalogForAPI(in []StoreCatalogItem) []StoreCatalogItem {
	out := make([]StoreCatalogItem, 0, len(in))
	for _, item := range in {
		copyItem := item
		copyItem.Content = ""
		priceMap := make(map[string]string, len(item.Prices))
		for asset, price := range item.Prices {
			priceMap[asset] = price
		}
		copyItem.Prices = priceMap
		out = append(out, copyItem)
	}
	return out
}

func sortedAssets(homeBlockchains map[string]uint64) []string {
	out := make([]string, 0, len(homeBlockchains))
	for asset := range homeBlockchains {
		out = append(out, strings.ToLower(strings.TrimSpace(asset)))
	}
	sort.Strings(out)
	return out
}

func parsePositiveAmount(raw string) (decimal.Decimal, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil {
		return decimal.Decimal{}, invalidf("invalid amount")
	}
	if !amount.IsPositive() {
		return decimal.Decimal{}, invalidf("amount must be positive")
	}
	return amount, nil
}

func balancesForAsset(allocations []app.AppAllocationV1, userAddress string, appAddress string, asset string) (decimal.Decimal, decimal.Decimal) {
	userAmount := decimal.Zero
	appAmount := decimal.Zero
	for _, allocation := range allocations {
		if !strings.EqualFold(allocation.Asset, asset) {
			continue
		}
		switch {
		case strings.EqualFold(allocation.Participant, userAddress):
			userAmount = allocation.Amount
		case strings.EqualFold(allocation.Participant, appAddress):
			appAmount = allocation.Amount
		}
	}
	return userAmount, appAmount
}

func openClosedStatus(isClosed bool) string {
	if isClosed {
		return "closed"
	}
	return "open"
}

func mustJSONString(value any) string {
	payload, _ := json.Marshal(value)
	return string(payload)
}

func seededCatalog() []StoreCatalogItem {
	return []StoreCatalogItem{
		{
			ID:          "book-go-101",
			Title:       "Go Systems Handbook",
			Description: "A practical field guide to Go services, interfaces, and runtime behavior.",
			Type:        "book",
			Prices: map[string]string{
				"yellow": "4.50",
				"yusd":   "2.50",
			},
			Content: "Go Systems Handbook\n\nThis sample chapter walks through interfaces, runtime lifecycles, and debugging patterns for long-running services.",
		},
		{
			ID:          "magazine-state-channels",
			Title:       "State Channel Monthly",
			Description: "A lightweight magazine issue about channels, signatures, and settlement UX.",
			Type:        "magazine",
			Prices: map[string]string{
				"yellow": "2.00",
				"yusd":   "1.00",
			},
			Content: "State Channel Monthly\n\nFeature stories on settlement rails, app sessions, and why product UX should hide protocol noise.",
		},
		{
			ID:          "article-micropayments",
			Title:       "Designing Instant Micropayments",
			Description: "Short-form reading on instant purchases and content gating with app sessions.",
			Type:        "article",
			Prices: map[string]string{
				"yellow": "0.75",
				"yusd":   "0.50",
			},
			Content: "Designing Instant Micropayments\n\nMicropayment UX succeeds when top-level product actions stay simple and settlement details remain observable but hidden.",
		},
	}
}
