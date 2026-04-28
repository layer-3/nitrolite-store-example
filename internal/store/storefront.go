package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type WalletStoreSession struct {
	WalletAddress  string
	Asset          string
	AppSessionID   string
	Status         string
	Version        uint64
	UserAllocation string
	AppAllocation  string
	SessionData    string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type WalletPurchase struct {
	ID            string    `json:"id"`
	WalletAddress string    `json:"wallet_address"`
	ItemID        string    `json:"item_id"`
	Asset         string    `json:"asset"`
	AppSessionID  string    `json:"app_session_id"`
	Version       uint64    `json:"version"`
	Status        string    `json:"status"`
	SessionData   string    `json:"session_data"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	PurchasedAt   time.Time `json:"purchased_at"`
}

const (
	WalletPurchaseStatusPending   = "pending"
	WalletPurchaseStatusSubmitted = "submitted"
	WalletPurchaseStatusFailed    = "failed"
)

func (s *Store) UpsertWalletSession(ctx context.Context, session WalletStoreSession) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO wallet_store_sessions (
			wallet_address, asset, app_session_id, status, version, user_allocation, app_allocation, session_data, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(wallet_address, asset) DO UPDATE SET
			app_session_id = excluded.app_session_id,
			status = excluded.status,
			version = excluded.version,
			user_allocation = excluded.user_allocation,
			app_allocation = excluded.app_allocation,
			session_data = excluded.session_data,
			updated_at = excluded.updated_at
	`, session.WalletAddress, session.Asset, session.AppSessionID, session.Status, session.Version, session.UserAllocation, session.AppAllocation, session.SessionData, formatTime(session.CreatedAt), formatTime(session.UpdatedAt))
	return err
}

func (s *Store) GetWalletSession(ctx context.Context, walletAddress string, asset string) (*WalletStoreSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT wallet_address, asset, app_session_id, status, version, user_allocation, app_allocation, session_data, created_at, updated_at
		FROM wallet_store_sessions
		WHERE wallet_address = ? AND asset = ?
	`, walletAddress, asset)
	return scanWalletStoreSession(row)
}

func (s *Store) GetWalletSessionByAppSessionID(ctx context.Context, appSessionID string, asset string) (*WalletStoreSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT wallet_address, asset, app_session_id, status, version, user_allocation, app_allocation, session_data, created_at, updated_at
		FROM wallet_store_sessions
		WHERE app_session_id = ? AND asset = ?
	`, appSessionID, asset)
	return scanWalletStoreSession(row)
}

func (s *Store) UpsertPendingWalletPurchase(ctx context.Context, purchase WalletPurchase) error {
	createdAt := purchase.CreatedAt
	if createdAt.IsZero() {
		createdAt = purchase.PurchasedAt
	}
	updatedAt := purchase.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = purchase.PurchasedAt
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO wallet_purchases (
			id, wallet_address, item_id, asset, app_session_id, version, status, session_data, created_at, updated_at, purchased_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(wallet_address, item_id, asset) DO UPDATE SET
			app_session_id = excluded.app_session_id,
			version = excluded.version,
			status = excluded.status,
			session_data = excluded.session_data,
			updated_at = excluded.updated_at,
			purchased_at = excluded.purchased_at
		WHERE wallet_purchases.status <> ?
	`, purchase.ID, purchase.WalletAddress, purchase.ItemID, purchase.Asset, purchase.AppSessionID, purchase.Version, WalletPurchaseStatusPending, purchase.SessionData, formatTime(createdAt), formatTime(updatedAt), formatTime(purchase.PurchasedAt), WalletPurchaseStatusSubmitted)
	if err != nil {
		if isSQLiteUniqueError(err) {
			return ErrConflict
		}
		return err
	}
	if changed, err := result.RowsAffected(); err == nil && changed == 0 {
		return ErrConflict
	}
	return nil
}

func (s *Store) MarkWalletPurchaseSubmitted(ctx context.Context, id string, purchasedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE wallet_purchases
		SET status = ?, purchased_at = ?, updated_at = ?
		WHERE id = ?
	`, WalletPurchaseStatusSubmitted, formatTime(purchasedAt), formatTime(purchasedAt), id)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err == nil && changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkWalletPurchaseFailed(ctx context.Context, id string, updatedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE wallet_purchases
		SET status = ?, updated_at = ?
		WHERE id = ? AND status = ?
	`, WalletPurchaseStatusFailed, formatTime(updatedAt), id, WalletPurchaseStatusPending)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err == nil && changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) HasWalletPurchase(ctx context.Context, walletAddress string, itemID string, asset string) (bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM wallet_purchases WHERE wallet_address = ? AND item_id = ? AND asset = ? AND status = ? LIMIT 1
	`, walletAddress, itemID, asset, WalletPurchaseStatusSubmitted)
	var marker int
	if err := row.Scan(&marker); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) GetWalletPurchase(ctx context.Context, walletAddress string, itemID string, asset string) (*WalletPurchase, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, wallet_address, item_id, asset, app_session_id, version, status, session_data, created_at, updated_at, purchased_at
		FROM wallet_purchases
		WHERE wallet_address = ? AND item_id = ? AND asset = ?
	`, walletAddress, itemID, asset)
	return scanWalletPurchase(row)
}

func (s *Store) ListPendingWalletPurchases(ctx context.Context, walletAddress string, asset string) ([]WalletPurchase, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, wallet_address, item_id, asset, app_session_id, version, status, session_data, created_at, updated_at, purchased_at
		FROM wallet_purchases
		WHERE wallet_address = ? AND asset = ? AND status = ?
		ORDER BY updated_at ASC
	`, walletAddress, asset, WalletPurchaseStatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []WalletPurchase{}
	for rows.Next() {
		purchase, err := scanWalletPurchase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *purchase)
	}
	return out, rows.Err()
}

func (s *Store) ListPurchasesByWallet(ctx context.Context, walletAddress string, asset string) ([]WalletPurchase, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, wallet_address, item_id, asset, app_session_id, version, status, session_data, created_at, updated_at, purchased_at
		FROM wallet_purchases
		WHERE wallet_address = ? AND asset = ? AND status = ?
		ORDER BY purchased_at DESC
	`, walletAddress, asset, WalletPurchaseStatusSubmitted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []WalletPurchase{}
	for rows.Next() {
		purchase, err := scanWalletPurchase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *purchase)
	}
	return out, rows.Err()
}

func scanWalletStoreSession(scanner interface{ Scan(dest ...any) error }) (*WalletStoreSession, error) {
	var session WalletStoreSession
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(
		&session.WalletAddress,
		&session.Asset,
		&session.AppSessionID,
		&session.Status,
		&session.Version,
		&session.UserAllocation,
		&session.AppAllocation,
		&session.SessionData,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var err error
	session.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse wallet store session created_at: %w", err)
	}
	session.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse wallet store session updated_at: %w", err)
	}
	return &session, nil
}

func scanWalletPurchase(scanner interface{ Scan(dest ...any) error }) (*WalletPurchase, error) {
	var purchase WalletPurchase
	var createdAt string
	var updatedAt string
	var purchasedAt string
	if err := scanner.Scan(
		&purchase.ID,
		&purchase.WalletAddress,
		&purchase.ItemID,
		&purchase.Asset,
		&purchase.AppSessionID,
		&purchase.Version,
		&purchase.Status,
		&purchase.SessionData,
		&createdAt,
		&updatedAt,
		&purchasedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var err error
	purchase.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse wallet purchase created_at: %w", err)
	}
	purchase.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse wallet purchase updated_at: %w", err)
	}
	purchase.PurchasedAt, err = parseTime(purchasedAt)
	if err != nil {
		return nil, fmt.Errorf("parse wallet purchase purchased_at: %w", err)
	}
	return &purchase, nil
}

func WalletPurchaseID(walletAddress string, asset string, itemID string) string {
	return fmt.Sprintf("%s:%s:%s", walletAddress, asset, itemID)
}

func isSQLiteUniqueError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}
