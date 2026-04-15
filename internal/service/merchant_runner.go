package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/layer-3/nitrolite-go-example/internal/config"
	"github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite-go-example/internal/store"
	"github.com/shopspring/decimal"
)

type operationPayload struct {
	PaymentRequestID  string `json:"payment_request_id,omitempty"`
	OrderID           string `json:"order_id,omitempty"`
	PayoutID          string `json:"payout_id,omitempty"`
	SessionID         string `json:"session_id,omitempty"`
	Asset             string `json:"asset,omitempty"`
	Amount            string `json:"amount,omitempty"`
	DestinationWallet string `json:"destination_wallet,omitempty"`
	TxHash            string `json:"tx_hash,omitempty"`
	ChainID           uint64 `json:"chain_id,omitempty"`
	TargetVersion     uint64 `json:"target_version,omitempty"`
	Step              string `json:"step,omitempty"`
}

type receiptStatus string

const (
	receiptPending   receiptStatus = "pending"
	receiptConfirmed receiptStatus = "confirmed"
	receiptFailed    receiptStatus = "failed"
)

type chainReceiptReader interface {
	ReceiptStatus(ctx context.Context, chainID uint64, txHash string) (receiptStatus, error)
}

type MerchantOperationRunner struct {
	store         *store.Store
	manager       clientProvider
	mutations     *MutationService
	appSessions   *AppSessionService
	receipts      chainReceiptReader
	merchantAppID string
	logger        *slog.Logger
	pollInterval  time.Duration
	now           func() time.Time
}

func NewMerchantOperationRunner(cfg *config.Config, store *store.Store, manager clientProvider, signer signing.Signer, logger *slog.Logger) *MerchantOperationRunner {
	return &MerchantOperationRunner{
		store:         store,
		manager:       manager,
		mutations:     NewMutationService(manager, cfg.HomeBlockchains),
		appSessions:   NewAppSessionService(manager, signer, nil),
		receipts:      newRPCReceiptReader(cfg.BlockchainRPCURLs),
		merchantAppID: strings.TrimSpace(cfg.MerchantAppID),
		logger:        logger,
		pollInterval:  time.Second,
		now:           time.Now,
	}
}

func (r *MerchantOperationRunner) Run(ctx context.Context) {
	if err := r.store.MarkRunningOperationsInterrupted(ctx); err != nil && r.logger != nil {
		r.logger.Error("failed to mark interrupted operations", "error", err)
	}

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		processed, err := r.processNext(ctx)
		if err != nil && r.logger != nil {
			r.logger.Error("merchant runner iteration failed", "error", err)
		}
		if processed {
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *MerchantOperationRunner) processNext(ctx context.Context) (bool, error) {
	ops, err := r.store.ListOperationsByStatuses(ctx, 1, "queued", "waiting_chain", "waiting_sync")
	if err != nil {
		return false, err
	}
	if len(ops) == 0 {
		return false, nil
	}
	return true, r.handleOperation(ctx, ops[0])
}

func (r *MerchantOperationRunner) handleOperation(ctx context.Context, op store.Operation) error {
	payload, err := unmarshalOperationPayload(op.Payload)
	if err != nil {
		return r.failOperation(ctx, op, payload, fmt.Sprintf("invalid operation payload: %v", err))
	}
	if op.Status == "queued" {
		now := r.now().UTC()
		op.Status = "running"
		op.StartedAt = &now
		op.ErrorMessage = ""
		if err := r.saveOperation(ctx, &op, payload); err != nil {
			return err
		}
	}

	switch op.Type {
	case operationTypeCapture:
		return r.runCapture(ctx, &op, payload)
	case operationTypeSettle, operationTypeRefund:
		return r.runResolution(ctx, &op, payload)
	case operationTypePayout:
		return r.runPayout(ctx, &op, payload)
	default:
		return r.failOperation(ctx, op, payload, "unsupported operation type")
	}
}

func (r *MerchantOperationRunner) runCapture(ctx context.Context, op *store.Operation, payload operationPayload) error {
	for {
		switch payload.Step {
		case "", "deposit_pending":
			chainID, err := r.homeChainForAsset(payload.Asset)
			if err != nil {
				return r.failOperation(ctx, *op, payload, err.Error())
			}
			amount, err := decimal.NewFromString(payload.Amount)
			if err != nil {
				return r.failOperation(ctx, *op, payload, "invalid amount in capture payload")
			}
			state, err := r.mutations.Deposit(ctx, chainID, payload.Asset, amount)
			if err != nil {
				return r.failOperation(ctx, *op, payload, err.Error())
			}
			payload.ChainID = chainID
			payload.TargetVersion = state.Version
			payload.Step = "checkpoint_pending"
			if err := r.saveOperation(ctx, op, payload); err != nil {
				return err
			}

		case "checkpoint_pending":
			txHash, err := r.mutations.Checkpoint(ctx, payload.Asset)
			if err != nil {
				return r.failOperation(ctx, *op, payload, err.Error())
			}
			payload.TxHash = txHash
			payload.Step = "checkpoint_submitted"
			op.Status = "waiting_chain"
			return r.saveOperation(ctx, op, payload)

		case "checkpoint_submitted":
			status, err := r.receipts.ReceiptStatus(ctx, payload.ChainID, payload.TxHash)
			if err != nil {
				return err
			}
			if status == receiptPending {
				return nil
			}
			if status == receiptFailed {
				return r.failOperation(ctx, *op, payload, "checkpoint transaction reverted")
			}
			payload.Step = "channel_sync_pending"
			op.Status = "waiting_sync"
			return r.saveOperation(ctx, op, payload)

		case "channel_sync_pending":
			synced, err := r.channelReachedVersion(ctx, payload.Asset, payload.TargetVersion)
			if err != nil {
				return err
			}
			if !synced {
				return nil
			}
			payload.Step = "session_create_pending"
			op.Status = "running"
			if err := r.saveOperation(ctx, op, payload); err != nil {
				return err
			}

		case "session_create_pending":
			created, err := r.appSessions.CreateEmptySession(ctx, r.merchantAppID, fmt.Sprintf(`{"order_id":"%s"}`, payload.OrderID))
			if err != nil {
				return r.failOperation(ctx, *op, payload, err.Error())
			}
			payload.SessionID = created.SessionID
			payload.Step = "session_deposit_pending"

			order, err := r.store.GetOrder(ctx, payload.OrderID)
			if err != nil {
				return err
			}
			order.AppSessionID = created.SessionID
			order.UpdatedAt = r.now().UTC()
			if err := r.store.UpdateOrder(ctx, *order); err != nil {
				return fmt.Errorf("failed to persist order session: %w", err)
			}
			if err := r.saveOperation(ctx, op, payload); err != nil {
				return err
			}

		case "session_deposit_pending":
			amount, err := decimal.NewFromString(payload.Amount)
			if err != nil {
				return r.failOperation(ctx, *op, payload, "invalid amount in capture payload")
			}
			result, err := r.appSessions.DepositSession(ctx, payload.SessionID, DepositAppSessionRequest{
				Asset:  payload.Asset,
				Amount: amount,
			})
			if err != nil {
				return r.failOperation(ctx, *op, payload, err.Error())
			}
			payload.TargetVersion = result.Version
			payload.Step = "session_sync_pending"
			op.Status = "waiting_sync"
			return r.saveOperation(ctx, op, payload)

		case "session_sync_pending":
			synced, err := r.sessionReachedVersion(ctx, payload.SessionID, payload.TargetVersion, false)
			if err != nil {
				return err
			}
			if !synced {
				return nil
			}

			order, err := r.store.GetOrder(ctx, payload.OrderID)
			if err != nil {
				return err
			}
			order.Status = "reserved"
			order.UpdatedAt = r.now().UTC()
			if err := r.store.UpdateOrder(ctx, *order); err != nil {
				return err
			}
			if err := r.store.UpdatePaymentRequestState(ctx, payload.PaymentRequestID, "completed", order.ID, op.ID); err != nil {
				return err
			}
			return r.completeOperation(ctx, op, payload)

		default:
			return r.failOperation(ctx, *op, payload, "unsupported capture step")
		}
	}
}

func (r *MerchantOperationRunner) runResolution(ctx context.Context, op *store.Operation, payload operationPayload) error {
	for {
		switch payload.Step {
		case "", "session_close_pending":
			result, err := r.appSessions.CloseSession(ctx, payload.SessionID)
			if err != nil {
				return r.failOperation(ctx, *op, payload, err.Error())
			}
			payload.TargetVersion = result.Version
			payload.Step = "session_close_sync_pending"
			op.Status = "waiting_sync"
			return r.saveOperation(ctx, op, payload)

		case "session_close_sync_pending":
			synced, err := r.sessionReachedVersion(ctx, payload.SessionID, payload.TargetVersion, true)
			if err != nil {
				return err
			}
			if !synced {
				return nil
			}

			order, err := r.store.GetOrder(ctx, payload.OrderID)
			if err != nil {
				return err
			}
			if op.Type == operationTypeSettle {
				order.Status = "settled"
			} else {
				order.Status = "refunded"
			}
			order.UpdatedAt = r.now().UTC()
			if err := r.store.UpdateOrder(ctx, *order); err != nil {
				return err
			}
			return r.completeOperation(ctx, op, payload)

		default:
			return r.failOperation(ctx, *op, payload, "unsupported resolution step")
		}
	}
}

func (r *MerchantOperationRunner) runPayout(ctx context.Context, op *store.Operation, payload operationPayload) error {
	for {
		switch payload.Step {
		case "", "withdraw_pending":
			chainID, err := r.homeChainForAsset(payload.Asset)
			if err != nil {
				return r.failOperation(ctx, *op, payload, err.Error())
			}
			amount, err := decimal.NewFromString(payload.Amount)
			if err != nil {
				return r.failOperation(ctx, *op, payload, "invalid payout amount")
			}
			state, err := r.mutations.Withdraw(ctx, chainID, payload.Asset, amount)
			if err != nil {
				return r.failOperation(ctx, *op, payload, err.Error())
			}
			payload.ChainID = chainID
			payload.TargetVersion = state.Version
			payload.Step = "payout_checkpoint_pending"
			if err := r.saveOperation(ctx, op, payload); err != nil {
				return err
			}

		case "payout_checkpoint_pending":
			txHash, err := r.mutations.Checkpoint(ctx, payload.Asset)
			if err != nil {
				return r.failOperation(ctx, *op, payload, err.Error())
			}
			payload.TxHash = txHash
			payload.Step = "payout_checkpoint_submitted"
			op.Status = "waiting_chain"
			return r.saveOperation(ctx, op, payload)

		case "payout_checkpoint_submitted":
			status, err := r.receipts.ReceiptStatus(ctx, payload.ChainID, payload.TxHash)
			if err != nil {
				return err
			}
			if status == receiptPending {
				return nil
			}
			if status == receiptFailed {
				return r.failOperation(ctx, *op, payload, "payout checkpoint transaction reverted")
			}
			payload.Step = "payout_sync_pending"
			op.Status = "waiting_sync"
			return r.saveOperation(ctx, op, payload)

		case "payout_sync_pending":
			synced, err := r.channelReachedVersion(ctx, payload.Asset, payload.TargetVersion)
			if err != nil {
				return err
			}
			if !synced {
				return nil
			}

			payout, err := r.store.GetPayout(ctx, payload.PayoutID)
			if err != nil {
				return err
			}
			payout.Status = "completed"
			payout.UpdatedAt = r.now().UTC()
			if err := r.store.UpdatePayout(ctx, *payout); err != nil {
				return err
			}
			return r.completeOperation(ctx, op, payload)

		default:
			return r.failOperation(ctx, *op, payload, "unsupported payout step")
		}
	}
}

func (r *MerchantOperationRunner) failOperation(ctx context.Context, op store.Operation, payload operationPayload, message string) error {
	now := r.now().UTC()
	op.Status = "failed"
	op.ErrorMessage = message
	op.CompletedAt = &now
	if err := r.saveOperation(ctx, &op, payload); err != nil {
		return err
	}

	switch op.Type {
	case operationTypeCapture:
		if payload.OrderID != "" {
			if order, err := r.store.GetOrder(ctx, payload.OrderID); err == nil {
				order.Status = "failed"
				order.UpdatedAt = now
				_ = r.store.UpdateOrder(ctx, *order)
			}
		}
		if payload.PaymentRequestID != "" {
			_ = r.store.UpdatePaymentRequestState(ctx, payload.PaymentRequestID, "failed", payload.OrderID, op.ID)
		}
	case operationTypeSettle, operationTypeRefund:
		if payload.OrderID != "" {
			if order, err := r.store.GetOrder(ctx, payload.OrderID); err == nil {
				order.Status = "reserved"
				order.UpdatedAt = now
				_ = r.store.UpdateOrder(ctx, *order)
			}
		}
	case operationTypePayout:
		if payload.PayoutID != "" {
			if payout, err := r.store.GetPayout(ctx, payload.PayoutID); err == nil {
				payout.Status = "failed"
				payout.UpdatedAt = now
				_ = r.store.UpdatePayout(ctx, *payout)
			}
		}
	}
	return nil
}

func (r *MerchantOperationRunner) completeOperation(ctx context.Context, op *store.Operation, payload operationPayload) error {
	now := r.now().UTC()
	op.Status = "completed"
	op.CompletedAt = &now
	op.ErrorMessage = ""
	return r.saveOperation(ctx, op, payload)
}

func (r *MerchantOperationRunner) saveOperation(ctx context.Context, op *store.Operation, payload operationPayload) error {
	encoded, err := marshalOperationPayload(payload)
	if err != nil {
		return err
	}
	op.Payload = encoded
	return r.store.UpdateOperation(ctx, *op)
}

func (r *MerchantOperationRunner) channelReachedVersion(ctx context.Context, asset string, version uint64) (bool, error) {
	client, wallet, err := activeClient(r.manager)
	if err != nil {
		return false, err
	}
	channel, err := client.GetHomeChannel(ctx, wallet, strings.ToLower(strings.TrimSpace(asset)))
	if err != nil {
		return false, nil
	}
	return channel != nil && channel.StateVersion >= version, nil
}

func (r *MerchantOperationRunner) sessionReachedVersion(ctx context.Context, sessionID string, version uint64, closed bool) (bool, error) {
	session, err := r.appSessions.GetSession(ctx, sessionID)
	if err != nil {
		return false, nil
	}
	if closed && !session.Session.IsClosed {
		return false, nil
	}
	return session.Session.Version >= version, nil
}

func (r *MerchantOperationRunner) homeChainForAsset(asset string) (uint64, error) {
	normalized, err := normalizeAsset(asset)
	if err != nil {
		return 0, err
	}
	chainID, ok := r.mutations.homeBlockchains[normalized]
	if !ok {
		return 0, invalidf("asset %s is not configured in HOME_BLOCKCHAINS", normalized)
	}
	return chainID, nil
}

func marshalOperationPayload(payload operationPayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal operation payload: %w", err)
	}
	return string(raw), nil
}

func unmarshalOperationPayload(raw string) (operationPayload, error) {
	if strings.TrimSpace(raw) == "" {
		return operationPayload{}, nil
	}
	var payload operationPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return operationPayload{}, err
	}
	return payload, nil
}

type rpcReceiptReader struct {
	urls    map[uint64]string
	clients map[uint64]*ethclient.Client
	mu      sync.Mutex
}

func newRPCReceiptReader(blockchainRPCURLs map[string]string) chainReceiptReader {
	urls := make(map[uint64]string, len(blockchainRPCURLs))
	for chainID, rawURL := range blockchainRPCURLs {
		parsed, err := strconv.ParseUint(chainID, 10, 64)
		if err != nil {
			continue
		}
		urls[parsed] = rawURL
	}
	return &rpcReceiptReader{
		urls:    urls,
		clients: make(map[uint64]*ethclient.Client),
	}
}

func (r *rpcReceiptReader) ReceiptStatus(ctx context.Context, chainID uint64, txHash string) (receiptStatus, error) {
	client, err := r.clientForChain(ctx, chainID)
	if err != nil {
		return receiptPending, err
	}
	receipt, err := client.TransactionReceipt(ctx, common.HexToHash(txHash))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return receiptPending, nil
		}
		return receiptPending, err
	}
	if receipt.Status == 1 {
		return receiptConfirmed, nil
	}
	return receiptFailed, nil
}

func (r *rpcReceiptReader) clientForChain(ctx context.Context, chainID uint64) (*ethclient.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if client, ok := r.clients[chainID]; ok {
		return client, nil
	}
	rawURL, ok := r.urls[chainID]
	if !ok {
		return nil, fmt.Errorf("no rpc configured for chain %d", chainID)
	}
	client, err := ethclient.DialContext(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	r.clients[chainID] = client
	return client, nil
}
