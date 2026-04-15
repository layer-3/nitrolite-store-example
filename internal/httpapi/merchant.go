package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/layer-3/nitrolite-go-example/internal/service"
	"github.com/layer-3/nitrolite-go-example/internal/store"
	"github.com/shopspring/decimal"
)

type createPaymentRequestBody struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Asset       string `json:"asset"`
	Amount      string `json:"amount"`
}

type payoutBody struct {
	Asset             string `json:"asset"`
	Amount            string `json:"amount"`
	DestinationWallet string `json:"destination_wallet"`
}

type leaseStatusResponse struct {
	Held                  bool   `json:"held"`
	OwnedByCurrentSession bool   `json:"owned_by_current_session"`
	AcquiredAt            string `json:"acquired_at,omitempty"`
	HeartbeatAt           string `json:"heartbeat_at,omitempty"`
	ExpiresAt             string `json:"expires_at,omitempty"`
}

type paymentRequestResponse struct {
	PaymentRequestID string `json:"payment_request_id"`
	Slug             string `json:"slug"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Asset            string `json:"asset"`
	Amount           string `json:"amount"`
	Status           string `json:"status"`
	OrderID          string `json:"order_id,omitempty"`
	OperationID      string `json:"operation_id,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type orderResponse struct {
	OrderID          string `json:"order_id"`
	PaymentRequestID string `json:"payment_request_id"`
	OperationID      string `json:"operation_id"`
	AppSessionID     string `json:"app_session_id,omitempty"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Asset            string `json:"asset"`
	Amount           string `json:"amount"`
	Status           string `json:"status"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type payoutResponse struct {
	PayoutID          string `json:"payout_id"`
	OperationID       string `json:"operation_id"`
	Asset             string `json:"asset"`
	Amount            string `json:"amount"`
	DestinationWallet string `json:"destination_wallet"`
	Status            string `json:"status"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

type operationResponse struct {
	OperationID  string  `json:"operation_id"`
	Type         string  `json:"type"`
	ResourceID   string  `json:"resource_id"`
	Status       string  `json:"status"`
	ErrorMessage string  `json:"error_message,omitempty"`
	Payload      any     `json:"payload,omitempty"`
	QueuedAt     string  `json:"queued_at"`
	StartedAt    *string `json:"started_at,omitempty"`
	CompletedAt  *string `json:"completed_at,omitempty"`
}

func dashboardOverviewHandler(dashboard *service.MerchantDashboardService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		overview, err := dashboard.GetOverview(r.Context(), r.URL.Query().Get("asset"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		payload := map[string]any{
			"merchant_name":    overview.MerchantName,
			"status":           encodeHealth(overview.Health),
			"wallet":           walletResponse{Address: overview.WalletAddress, HomeBlockchains: overview.HomeBlockchains},
			"selected_asset":   overview.SelectedAsset,
			"assets":           encodeAssets(overview.Assets),
			"balances":         encodeBalances(overview.Balances),
			"payment_requests": encodePaymentRequests(overview.PaymentRequests),
			"orders":           encodeOrders(overview.Orders),
			"payouts":          encodePayouts(overview.Payouts),
			"operations":       encodeOperations(overview.Operations),
			"summary": map[string]any{
				"available_balance": overview.Summary.AvailableBalance,
				"reserved_balance":  overview.Summary.ReservedBalance,
				"pending_count":     overview.Summary.PendingCount,
				"open_orders":       overview.Summary.OpenOrders,
			},
		}
		if overview.Channel != nil {
			encoded := encodeChannel(overview.Channel)
			payload["channel"] = encoded
		}
		if overview.LatestState != nil {
			encoded := encodeState(overview.LatestState)
			payload["latest_state"] = encoded
		}
		if overview.LatestActivity != nil {
			encoded := encodeTransaction(*overview.LatestActivity)
			payload["latest_activity"] = encoded
		}
		if overview.Lease != nil {
			payload["lease"] = encodeLeaseStatus(overview.Lease, tokenOwnsLease(r, overview.Lease))
		}
		writeJSON(w, http.StatusOK, payload)
	}
}

func createPaymentRequestHandler(merchant *service.MerchantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body createPaymentRequestBody
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		amount, err := decimal.NewFromString(strings.TrimSpace(body.Amount))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "amount must be a decimal string")
			return
		}
		result, err := merchant.CreatePaymentRequest(r.Context(), service.CreatePaymentRequestRequest{
			Title:       body.Title,
			Description: body.Description,
			Asset:       body.Asset,
			Amount:      amount,
		}, requestBaseURL(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}

		response := encodePaymentRequest(result.PaymentRequest)
		writeJSON(w, http.StatusCreated, map[string]any{
			"payment_request_id": response.PaymentRequestID,
			"slug":               response.Slug,
			"pay_url":            result.PayURL,
			"status":             response.Status,
			"payment_request":    response,
		})
	}
}

func paymentRequestPageHandler(merchant *service.MerchantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		page, err := merchant.GetPaymentRequestPage(r.Context(), slug)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		payload := map[string]any{
			"merchant_name":   page.MerchantName,
			"payment_request": encodePaymentRequest(page.PaymentRequest),
		}
		if page.Order != nil {
			payload["order"] = encodeOrder(*page.Order)
		}
		if page.Operation != nil {
			payload["operation"] = encodeOperation(*page.Operation)
		}
		writeJSON(w, http.StatusOK, payload)
	}
}

func payPaymentRequestHandler(merchant *service.MerchantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := merchant.PayPaymentRequest(r.Context(), r.PathValue("slug"))
		if err != nil {
			writeServiceError(w, err)
			return
		}

		status := http.StatusAccepted
		if !result.Created {
			status = http.StatusOK
		}
		writeJSON(w, status, map[string]any{
			"operation_id": result.Operation.ID,
			"resource_id":  result.ResourceID,
			"status":       result.Operation.Status,
		})
	}
}

func ordersHandler(appStore *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orders, err := appStore.ListOrders(r.Context(), 50)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to list orders")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"orders": encodeOrders(orders)})
	}
}

func orderDetailHandler(appStore *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		order, err := appStore.GetOrder(r.Context(), r.PathValue("id"))
		if err != nil {
			if err == store.ErrNotFound {
				writeError(w, http.StatusUnprocessableEntity, "not_found", "order not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load order")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"order": encodeOrder(*order)})
	}
}

func settleOrderHandler(merchant *service.MerchantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := merchant.QueueOrderResolution(r.Context(), r.PathValue("id"), "settle")
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"operation_id": result.Operation.ID,
			"resource_id":  result.ResourceID,
			"status":       result.Operation.Status,
		})
	}
}

func refundOrderHandler(merchant *service.MerchantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := merchant.QueueOrderResolution(r.Context(), r.PathValue("id"), "refund")
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"operation_id": result.Operation.ID,
			"resource_id":  result.ResourceID,
			"status":       result.Operation.Status,
		})
	}
}

func payoutsHandler(appStore *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payouts, err := appStore.ListPayouts(r.Context(), 50)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to list payouts")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"payouts": encodePayouts(payouts)})
	}
}

func createPayoutHandler(merchant *service.MerchantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body payoutBody
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		amount, err := decimal.NewFromString(strings.TrimSpace(body.Amount))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "amount must be a decimal string")
			return
		}
		result, err := merchant.QueuePayout(r.Context(), service.PayoutRequest{
			Asset:             body.Asset,
			Amount:            amount,
			DestinationWallet: body.DestinationWallet,
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"operation_id": result.Operation.ID,
			"resource_id":  result.ResourceID,
			"status":       result.Operation.Status,
		})
	}
}

func operationHandler(merchant *service.MerchantService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		op, err := merchant.GetOperation(r.Context(), r.PathValue("id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"operation": encodeOperation(*op)})
	}
}

func leaseStatusHandler(leases *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lease, err := leases.GetLease(r.Context())
		if err != nil {
			if err == store.ErrNotFound {
				writeJSON(w, http.StatusOK, leaseStatusResponse{Held: false})
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to read operator lease")
			return
		}
		writeJSON(w, http.StatusOK, encodeLeaseStatus(lease, tokenOwnsLease(r, lease)))
	}
}

func leaseAcquireHandler(leases *store.Store, ttl time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := readWriteSession(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_request", "lease acquisition requires a browser write session")
			return
		}
		lease, err := leases.AcquireLease(r.Context(), token, ttl)
		if err != nil {
			if err == store.ErrConflict {
				writeError(w, http.StatusConflict, "conflict", "operator lease held by another session")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to acquire operator lease")
			return
		}
		writeJSON(w, http.StatusOK, encodeLeaseStatus(lease, true))
	}
}

func leaseReleaseHandler(leases *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, _ := readWriteSession(r)
		if err := leases.ReleaseLease(r.Context(), token); err != nil && err != store.ErrNotFound {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to release operator lease")
			return
		}
		writeJSON(w, http.StatusOK, leaseStatusResponse{Held: false})
	}
}

func leaseHeartbeatHandler(leases *store.Store, ttl time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, _ := readWriteSession(r)
		lease, err := leases.HeartbeatLease(r.Context(), token, ttl)
		if err != nil {
			if err == store.ErrNotFound {
				writeError(w, http.StatusConflict, "conflict", "operator lease not held by this session")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to heartbeat operator lease")
			return
		}
		writeJSON(w, http.StatusOK, encodeLeaseStatus(lease, true))
	}
}

func encodeLeaseStatus(lease *store.OperatorLease, owned bool) leaseStatusResponse {
	if lease == nil {
		return leaseStatusResponse{Held: false}
	}
	return leaseStatusResponse{
		Held:                  true,
		OwnedByCurrentSession: owned,
		AcquiredAt:            lease.AcquiredAt.UTC().Format(time.RFC3339),
		HeartbeatAt:           lease.HeartbeatAt.UTC().Format(time.RFC3339),
		ExpiresAt:             lease.ExpiresAt.UTC().Format(time.RFC3339),
	}
}

func encodePaymentRequests(items []store.PaymentRequest) []paymentRequestResponse {
	out := make([]paymentRequestResponse, 0, len(items))
	for _, item := range items {
		out = append(out, encodePaymentRequest(item))
	}
	return out
}

func encodePaymentRequest(item store.PaymentRequest) paymentRequestResponse {
	return paymentRequestResponse{
		PaymentRequestID: item.ID,
		Slug:             item.Slug,
		Title:            item.Title,
		Description:      item.Description,
		Asset:            item.Asset,
		Amount:           item.Amount,
		Status:           item.Status,
		OrderID:          item.OrderID,
		OperationID:      item.OperationID,
		CreatedAt:        item.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        item.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func encodeOrders(items []store.Order) []orderResponse {
	out := make([]orderResponse, 0, len(items))
	for _, item := range items {
		out = append(out, encodeOrder(item))
	}
	return out
}

func encodeOrder(item store.Order) orderResponse {
	return orderResponse{
		OrderID:          item.ID,
		PaymentRequestID: item.PaymentRequestID,
		OperationID:      item.OperationID,
		AppSessionID:     item.AppSessionID,
		Title:            item.Title,
		Description:      item.Description,
		Asset:            item.Asset,
		Amount:           item.Amount,
		Status:           item.Status,
		CreatedAt:        item.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        item.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func encodePayouts(items []store.Payout) []payoutResponse {
	out := make([]payoutResponse, 0, len(items))
	for _, item := range items {
		out = append(out, encodePayout(item))
	}
	return out
}

func encodePayout(item store.Payout) payoutResponse {
	return payoutResponse{
		PayoutID:          item.ID,
		OperationID:       item.OperationID,
		Asset:             item.Asset,
		Amount:            item.Amount,
		DestinationWallet: item.DestinationWallet,
		Status:            item.Status,
		CreatedAt:         item.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:         item.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func encodeOperations(items []store.Operation) []operationResponse {
	out := make([]operationResponse, 0, len(items))
	for _, item := range items {
		out = append(out, encodeOperation(item))
	}
	return out
}

func encodeOperation(item store.Operation) operationResponse {
	var payload any
	if parsed, err := decodeOperationPayload(item.Payload); err == nil {
		payload = parsed
	}
	var startedAt *string
	if item.StartedAt != nil {
		value := item.StartedAt.UTC().Format(time.RFC3339)
		startedAt = &value
	}
	var completedAt *string
	if item.CompletedAt != nil {
		value := item.CompletedAt.UTC().Format(time.RFC3339)
		completedAt = &value
	}
	return operationResponse{
		OperationID:  item.ID,
		Type:         item.Type,
		ResourceID:   item.ResourceID,
		Status:       item.Status,
		ErrorMessage: item.ErrorMessage,
		Payload:      payload,
		QueuedAt:     item.QueuedAt.UTC().Format(time.RFC3339),
		StartedAt:    startedAt,
		CompletedAt:  completedAt,
	}
}

func tokenOwnsLease(r *http.Request, lease *store.OperatorLease) bool {
	if lease == nil {
		return false
	}
	token, ok := readWriteSession(r)
	return ok && token == lease.SessionToken
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if requestIsSecure(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func decodeOperationPayload(raw string) (any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}
