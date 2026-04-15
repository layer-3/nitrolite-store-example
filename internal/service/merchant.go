package service

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-go-example/internal/store"
	"github.com/layer-3/nitrolite/pkg/core"
	sdk "github.com/layer-3/nitrolite/sdk/go"
	"github.com/shopspring/decimal"
)

const (
	operationTypeCapture = "capture"
	operationTypeSettle  = "settle"
	operationTypeRefund  = "refund"
	operationTypePayout  = "payout"
)

type CreatePaymentRequestRequest struct {
	Title       string
	Description string
	Asset       string
	Amount      decimal.Decimal
}

type CreatePaymentRequestResult struct {
	PaymentRequest store.PaymentRequest
	PayURL         string
}

type PayoutRequest struct {
	Asset             string
	Amount            decimal.Decimal
	DestinationWallet string
}

type OperationDispatchResult struct {
	Operation  store.Operation
	ResourceID string
	Created    bool
}

type PaymentRequestPage struct {
	MerchantName   string
	PaymentRequest store.PaymentRequest
	Order          *store.Order
	Operation      *store.Operation
}

type DashboardSummary struct {
	AvailableBalance string
	ReservedBalance  string
	PendingCount     int
	OpenOrders       int
}

type MerchantDashboardOverview struct {
	MerchantName    string
	SelectedAsset   string
	WalletAddress   string
	HomeBlockchains map[string]uint64
	Health          nitrolite.Health
	Assets          []core.Asset
	Balances        []core.BalanceEntry
	Channel         *core.Channel
	LatestState     *core.State
	LatestActivity  *core.Transaction
	Lease           *store.OperatorLease
	PaymentRequests []store.PaymentRequest
	Orders          []store.Order
	Payouts         []store.Payout
	Operations      []store.Operation
	Summary         DashboardSummary
}

type MerchantService struct {
	store           *store.Store
	merchantName    string
	merchantAppID   string
	homeBlockchains map[string]uint64
	now             func() time.Time
}

func NewMerchantService(store *store.Store, merchantName string, merchantAppID string, homeBlockchains map[string]uint64) *MerchantService {
	normalized := make(map[string]uint64, len(homeBlockchains))
	for asset, chainID := range homeBlockchains {
		normalized[strings.ToLower(strings.TrimSpace(asset))] = chainID
	}
	return &MerchantService{
		store:           store,
		merchantName:    merchantName,
		merchantAppID:   strings.TrimSpace(merchantAppID),
		homeBlockchains: normalized,
		now:             time.Now,
	}
}

func (s *MerchantService) CreatePaymentRequest(ctx context.Context, req CreatePaymentRequestRequest, baseURL string) (*CreatePaymentRequestResult, error) {
	title := strings.TrimSpace(req.Title)
	description := strings.TrimSpace(req.Description)
	asset, err := normalizeAsset(req.Asset)
	if err != nil {
		return nil, err
	}
	if title == "" {
		return nil, invalidf("title is required")
	}
	if !req.Amount.IsPositive() {
		return nil, invalidf("amount must be positive")
	}
	if _, ok := s.homeBlockchains[asset]; !ok {
		return nil, invalidf("asset %s is not configured in HOME_BLOCKCHAINS", asset)
	}

	now := s.now().UTC()
	slug, err := newSlug()
	if err != nil {
		return nil, fmt.Errorf("failed to create payment slug: %w", err)
	}
	paymentRequest := store.PaymentRequest{
		ID:          uuid.NewString(),
		Slug:        slug,
		Title:       title,
		Description: description,
		Asset:       asset,
		Amount:      req.Amount.String(),
		Status:      "pending",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.CreatePaymentRequest(ctx, paymentRequest); err != nil {
		return nil, fmt.Errorf("failed to create payment request: %w", err)
	}
	payURL, err := paymentURL(baseURL, slug)
	if err != nil {
		return nil, err
	}
	return &CreatePaymentRequestResult{
		PaymentRequest: paymentRequest,
		PayURL:         payURL,
	}, nil
}

func (s *MerchantService) GetPaymentRequestPage(ctx context.Context, slug string) (*PaymentRequestPage, error) {
	paymentRequest, err := s.store.GetPaymentRequestBySlug(ctx, strings.TrimSpace(slug))
	if err != nil {
		if err == store.ErrNotFound {
			return nil, notFoundf("payment request not found")
		}
		return nil, fmt.Errorf("failed to load payment request: %w", err)
	}

	var order *store.Order
	if paymentRequest.OrderID != "" {
		order, err = s.store.GetOrder(ctx, paymentRequest.OrderID)
		if err != nil && err != store.ErrNotFound {
			return nil, fmt.Errorf("failed to load order: %w", err)
		}
		if err == store.ErrNotFound {
			order = nil
		}
	}

	var operation *store.Operation
	if paymentRequest.OperationID != "" {
		operation, err = s.store.GetOperation(ctx, paymentRequest.OperationID)
		if err != nil && err != store.ErrNotFound {
			return nil, fmt.Errorf("failed to load operation: %w", err)
		}
		if err == store.ErrNotFound {
			operation = nil
		}
	}

	return &PaymentRequestPage{
		MerchantName:   s.merchantName,
		PaymentRequest: *paymentRequest,
		Order:          order,
		Operation:      operation,
	}, nil
}

func (s *MerchantService) PayPaymentRequest(ctx context.Context, slug string) (*OperationDispatchResult, error) {
	now := s.now().UTC()
	order := store.Order{
		ID:               uuid.NewString(),
		PaymentRequestID: "",
		OperationID:      uuid.NewString(),
		AppSessionID:     "",
		Status:           "capturing",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	op := store.Operation{
		ID:          order.OperationID,
		Type:        operationTypeCapture,
		ResourceID:  order.ID,
		Status:      "queued",
		Payload:     "{}",
		QueuedAt:    now,
		StartedAt:   nil,
		CompletedAt: nil,
	}

	paymentRequest, existingOrder, existingOp, created, err := s.store.BeginPaymentRequestProcessing(ctx, strings.TrimSpace(slug), order, op)
	if err != nil {
		switch err {
		case store.ErrNotFound:
			return nil, notFoundf("payment request not found")
		case store.ErrConflict:
			return nil, conflictf("payment request is no longer payable")
		default:
			return nil, fmt.Errorf("failed to start payment processing: %w", err)
		}
	}

	if !created {
		result := &OperationDispatchResult{Created: false}
		if existingOp != nil {
			result.Operation = *existingOp
		}
		if existingOrder != nil {
			result.ResourceID = existingOrder.ID
		}
		return result, nil
	}

	order.PaymentRequestID = paymentRequest.ID
	order.Title = paymentRequest.Title
	order.Description = paymentRequest.Description
	order.Asset = paymentRequest.Asset
	order.Amount = paymentRequest.Amount

	payload, err := marshalOperationPayload(operationPayload{
		PaymentRequestID: paymentRequest.ID,
		OrderID:          order.ID,
		Asset:            paymentRequest.Asset,
		Amount:           paymentRequest.Amount,
		Step:             "deposit_pending",
	})
	if err != nil {
		return nil, err
	}
	op.Payload = payload
	if err := s.store.UpdateOperation(ctx, op); err != nil {
		return nil, fmt.Errorf("failed to persist operation payload: %w", err)
	}

	return &OperationDispatchResult{
		Operation:  op,
		ResourceID: order.ID,
		Created:    true,
	}, nil
}

func (s *MerchantService) QueueOrderResolution(ctx context.Context, orderID string, operationType string) (*OperationDispatchResult, error) {
	order, err := s.store.GetOrder(ctx, strings.TrimSpace(orderID))
	if err != nil {
		if err == store.ErrNotFound {
			return nil, notFoundf("order not found")
		}
		return nil, fmt.Errorf("failed to load order: %w", err)
	}
	if order.AppSessionID == "" {
		return nil, conflictf("order has no settlement session")
	}
	if order.Status != "reserved" {
		return nil, conflictf("order must be reserved before it can be %s", operationType)
	}

	now := s.now().UTC()
	op := store.Operation{
		ID:         uuid.NewString(),
		Type:       operationType,
		ResourceID: order.ID,
		Status:     "queued",
		Payload:    "{}",
		QueuedAt:   now,
	}
	payload, err := marshalOperationPayload(operationPayload{
		OrderID:   order.ID,
		SessionID: order.AppSessionID,
		Asset:     order.Asset,
		Amount:    order.Amount,
		Step:      "session_close_pending",
	})
	if err != nil {
		return nil, err
	}
	op.Payload = payload
	if err := s.store.CreateOperation(ctx, op); err != nil {
		return nil, fmt.Errorf("failed to create %s operation: %w", operationType, err)
	}

	order.OperationID = op.ID
	order.Status = map[string]string{
		operationTypeSettle: "settling",
		operationTypeRefund: "refunding",
	}[operationType]
	order.UpdatedAt = now
	if err := s.store.UpdateOrder(ctx, *order); err != nil {
		return nil, fmt.Errorf("failed to update order: %w", err)
	}

	return &OperationDispatchResult{
		Operation:  op,
		ResourceID: order.ID,
		Created:    true,
	}, nil
}

func (s *MerchantService) QueuePayout(ctx context.Context, req PayoutRequest) (*OperationDispatchResult, error) {
	asset, err := normalizeAsset(req.Asset)
	if err != nil {
		return nil, err
	}
	if _, ok := s.homeBlockchains[asset]; !ok {
		return nil, invalidf("asset %s is not configured in HOME_BLOCKCHAINS", asset)
	}
	if !req.Amount.IsPositive() {
		return nil, invalidf("amount must be positive")
	}
	destination := strings.TrimSpace(req.DestinationWallet)
	if !common.IsHexAddress(destination) {
		return nil, invalidf("invalid destination_wallet")
	}

	now := s.now().UTC()
	payout := store.Payout{
		ID:                uuid.NewString(),
		OperationID:       uuid.NewString(),
		Asset:             asset,
		Amount:            req.Amount.String(),
		DestinationWallet: destination,
		Status:            "pending",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	payload, err := marshalOperationPayload(operationPayload{
		PayoutID:          payout.ID,
		Asset:             asset,
		Amount:            payout.Amount,
		DestinationWallet: destination,
		Step:              "withdraw_pending",
	})
	if err != nil {
		return nil, err
	}
	op := store.Operation{
		ID:         payout.OperationID,
		Type:       operationTypePayout,
		ResourceID: payout.ID,
		Status:     "queued",
		Payload:    payload,
		QueuedAt:   now,
	}
	if err := s.store.CreatePayoutWithOperation(ctx, payout, op); err != nil {
		return nil, fmt.Errorf("failed to create payout: %w", err)
	}

	return &OperationDispatchResult{
		Operation:  op,
		ResourceID: payout.ID,
		Created:    true,
	}, nil
}

func (s *MerchantService) GetOperation(ctx context.Context, operationID string) (*store.Operation, error) {
	op, err := s.store.GetOperation(ctx, strings.TrimSpace(operationID))
	if err != nil {
		if err == store.ErrNotFound {
			return nil, notFoundf("operation not found")
		}
		return nil, fmt.Errorf("failed to load operation: %w", err)
	}
	return op, nil
}

type MerchantDashboardService struct {
	provider        clientProvider
	store           *store.Store
	merchantName    string
	homeBlockchains map[string]uint64
}

func NewMerchantDashboardService(provider clientProvider, store *store.Store, merchantName string, homeBlockchains map[string]uint64) *MerchantDashboardService {
	normalized := make(map[string]uint64, len(homeBlockchains))
	for asset, chainID := range homeBlockchains {
		normalized[strings.ToLower(strings.TrimSpace(asset))] = chainID
	}
	return &MerchantDashboardService{
		provider:        provider,
		store:           store,
		merchantName:    merchantName,
		homeBlockchains: normalized,
	}
}

func (s *MerchantDashboardService) GetOverview(ctx context.Context, requestedAsset string) (*MerchantDashboardOverview, error) {
	client, wallet, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	assets, err := client.GetAssets(ctx, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	balances, err := client.GetBalances(ctx, wallet)
	if err != nil {
		return nil, ErrUnavailable
	}
	selectedAsset := chooseSelectedAsset(requestedAsset, balances, assets, s.homeBlockchains)

	var channel *core.Channel
	if selectedAsset != "" {
		channel, err = client.GetHomeChannel(ctx, wallet, selectedAsset)
		if err != nil {
			return nil, ErrUnavailable
		}
	}

	var latestState *core.State
	if selectedAsset != "" {
		latestState, err = client.GetLatestState(ctx, wallet, selectedAsset, true)
		if err != nil {
			return nil, ErrUnavailable
		}
	}

	var latestActivity *core.Transaction
	offset := uint32(0)
	limit := uint32(10)
	transactions, _, err := client.GetTransactions(ctx, wallet, &sdk.GetTransactionsOptions{
		Pagination: &core.PaginationParams{Offset: &offset, Limit: &limit},
	})
	if err != nil {
		return nil, ErrUnavailable
	}
	if len(transactions) > 0 {
		latestActivity = &transactions[0]
	}

	paymentRequests, err := s.store.ListPaymentRequests(ctx, 8)
	if err != nil {
		return nil, fmt.Errorf("failed to list payment requests: %w", err)
	}
	orders, err := s.store.ListOrders(ctx, 12)
	if err != nil {
		return nil, fmt.Errorf("failed to list orders: %w", err)
	}
	payouts, err := s.store.ListPayouts(ctx, 8)
	if err != nil {
		return nil, fmt.Errorf("failed to list payouts: %w", err)
	}
	operations, err := s.store.ListOperations(ctx, 12)
	if err != nil {
		return nil, fmt.Errorf("failed to list operations: %w", err)
	}
	lease, err := s.store.GetLease(ctx)
	if err != nil && err != store.ErrNotFound {
		return nil, fmt.Errorf("failed to load lease: %w", err)
	}
	if err == store.ErrNotFound {
		lease = nil
	}

	summary := DashboardSummary{
		AvailableBalance: balanceString(balances, selectedAsset),
		ReservedBalance:  reservedBalance(orders, selectedAsset),
		PendingCount:     pendingOperationCount(operations),
		OpenOrders:       openOrderCount(orders),
	}

	return &MerchantDashboardOverview{
		MerchantName:    s.merchantName,
		SelectedAsset:   selectedAsset,
		WalletAddress:   wallet,
		HomeBlockchains: cloneHomeBlockchains(s.homeBlockchains),
		Health:          s.provider.Health(),
		Assets:          assets,
		Balances:        balances,
		Channel:         channel,
		LatestState:     latestState,
		LatestActivity:  latestActivity,
		Lease:           lease,
		PaymentRequests: paymentRequests,
		Orders:          orders,
		Payouts:         payouts,
		Operations:      operations,
		Summary:         summary,
	}, nil
}

func paymentURL(baseURL string, slug string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", invalidf("invalid base url")
	}
	parsed.Path = "/pay/" + slug
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func reservedBalance(orders []store.Order, asset string) string {
	total := decimal.Zero
	for _, order := range orders {
		if !strings.EqualFold(order.Asset, asset) {
			continue
		}
		switch order.Status {
		case "reserved", "settling", "refunding":
			value, err := decimal.NewFromString(order.Amount)
			if err == nil {
				total = total.Add(value)
			}
		}
	}
	return total.String()
}

func balanceString(balances []core.BalanceEntry, asset string) string {
	for _, balance := range balances {
		if strings.EqualFold(balance.Asset, asset) {
			return balance.Balance.String()
		}
	}
	return "0"
}

func pendingOperationCount(ops []store.Operation) int {
	count := 0
	for _, op := range ops {
		if op.Status != "completed" && op.Status != "failed" {
			count++
		}
	}
	return count
}

func openOrderCount(orders []store.Order) int {
	count := 0
	for _, order := range orders {
		if order.Status == "reserved" || order.Status == "capturing" || order.Status == "settling" || order.Status == "refunding" {
			count++
		}
	}
	return count
}

func newSlug() (string, error) {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(id) < 12 {
		return "", fmt.Errorf("failed to generate slug")
	}
	return strings.ToLower(id[:12]), nil
}

type byQueuedAt []store.Operation

func (s byQueuedAt) Len() int           { return len(s) }
func (s byQueuedAt) Less(i, j int) bool { return s[i].QueuedAt.Before(s[j].QueuedAt) }
func (s byQueuedAt) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func sortOperationsByQueuedAt(ops []store.Operation) {
	sort.Sort(byQueuedAt(ops))
}
