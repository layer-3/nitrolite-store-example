package httpapi

import (
	"net/http"

	"github.com/layer-3/nitrolite-go-example/internal/service"
	"github.com/shopspring/decimal"
)

type approveRequest struct {
	BlockchainID uint64          `json:"blockchain_id"`
	Asset        string          `json:"asset"`
	Amount       decimal.Decimal `json:"amount"`
}

type depositRequest struct {
	BlockchainID uint64          `json:"blockchain_id"`
	Asset        string          `json:"asset"`
	Amount       decimal.Decimal `json:"amount"`
}

type withdrawRequest struct {
	BlockchainID uint64          `json:"blockchain_id"`
	Asset        string          `json:"asset"`
	Amount       decimal.Decimal `json:"amount"`
}

type transferRequest struct {
	Recipient string          `json:"recipient"`
	Asset     string          `json:"asset"`
	Amount    decimal.Decimal `json:"amount"`
}

type checkpointRequest struct {
	Asset string `json:"asset"`
}

type closeChannelRequest struct {
	Asset string `json:"asset"`
}

type challengeRequest struct {
	Asset string `json:"asset"`
}

func approveHandler(mutations *service.MutationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req approveRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		txHash, err := mutations.ApproveToken(r.Context(), req.BlockchainID, req.Asset, req.Amount)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"tx_hash": txHash})
	}
}

func depositHandler(mutations *service.MutationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req depositRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		state, err := mutations.Deposit(r.Context(), req.BlockchainID, req.Asset, req.Amount)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"state":                encodeState(state),
			"ready_for_checkpoint": true,
		})
	}
}

func withdrawHandler(mutations *service.MutationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req withdrawRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		state, err := mutations.Withdraw(r.Context(), req.BlockchainID, req.Asset, req.Amount)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"state":                encodeState(state),
			"ready_for_checkpoint": true,
		})
	}
}

func transferHandler(mutations *service.MutationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req transferRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		state, err := mutations.Transfer(r.Context(), req.Recipient, req.Asset, req.Amount)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"state": encodeState(state),
			"note":  "off-chain only, no checkpoint needed",
		})
	}
}

func checkpointHandler(mutations *service.MutationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req checkpointRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		txHash, err := mutations.Checkpoint(r.Context(), req.Asset)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"tx_hash": txHash})
	}
}

func closeChannelHandler(channelService *service.ChannelService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req closeChannelRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		state, err := channelService.CloseChannel(r.Context(), service.CloseChannelRequest{Asset: req.Asset})
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"state": encodeState(state)})
	}
}

func challengeHandler(channelService *service.ChannelService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req challengeRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		result, err := channelService.ChallengeLatestState(r.Context(), service.ChallengeRequest{Asset: req.Asset})
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"tx_hash": result.TxHash})
	}
}
