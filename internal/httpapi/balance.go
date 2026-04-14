package httpapi

import (
	"net/http"

	"github.com/layer-3/nitrolite-go-example/internal/service"
)

func balancesHandler(balanceService *service.BalanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		balances, err := balanceService.GetBalances(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"balances": encodeBalances(balances),
		})
	}
}

func transactionsHandler(balanceService *service.BalanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := positiveUint32Query(r, "page", 1)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		perPage, err := positiveUint32Query(r, "per_page", 20)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		transactions, meta, err := balanceService.GetTransactions(r.Context(), service.TransactionsFilter{
			Asset:   optionalQuery(r, "asset"),
			Page:    page,
			PerPage: perPage,
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"transactions": encodeTransactions(transactions),
			"pagination":   encodePagination(meta),
		})
	}
}
