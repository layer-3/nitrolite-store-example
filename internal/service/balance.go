package service

import (
	"context"
	"fmt"

	"github.com/layer-3/nitrolite/pkg/core"
	sdk "github.com/layer-3/nitrolite/sdk/go"
)

// TransactionsFilter controls transaction list pagination and filtering.
type TransactionsFilter struct {
	Asset   *string
	Page    uint32
	PerPage uint32
}

// BalanceService exposes wallet balance and transaction reads.
type BalanceService struct {
	provider clientProvider
}

// NewBalanceService constructs a BalanceService.
func NewBalanceService(provider clientProvider) *BalanceService {
	return &BalanceService{provider: provider}
}

// GetBalances retrieves balances for the demo signer.
func (s *BalanceService) GetBalances(ctx context.Context) ([]core.BalanceEntry, error) {
	client, wallet, err := activeClient(s.provider)
	if err != nil {
		return nil, err
	}

	balances, err := client.GetBalances(ctx, wallet)
	if err != nil {
		return nil, fmt.Errorf("failed to get balances: %w", err)
	}
	return balances, nil
}

// GetTransactions retrieves transactions for the demo signer.
func (s *BalanceService) GetTransactions(ctx context.Context, filter TransactionsFilter) ([]core.Transaction, core.PaginationMetadata, error) {
	client, wallet, err := activeClient(s.provider)
	if err != nil {
		return nil, core.PaginationMetadata{}, err
	}

	offset := (filter.Page - 1) * filter.PerPage
	limit := filter.PerPage
	opts := &sdk.GetTransactionsOptions{
		Asset: filter.Asset,
		Pagination: &core.PaginationParams{
			Offset: &offset,
			Limit:  &limit,
		},
	}

	transactions, meta, err := client.GetTransactions(ctx, wallet, opts)
	if err != nil {
		return nil, core.PaginationMetadata{}, fmt.Errorf("failed to get transactions: %w", err)
	}
	return transactions, meta, nil
}
