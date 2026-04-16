package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type StoreSession struct {
	BrowserSessionID string
	Asset            string
	AppSessionID     string
	Status           string
	Version          uint64
	UserAllocation   string
	AppAllocation    string
	SessionData      string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Purchase struct {
	ID               string    `json:"id"`
	BrowserSessionID string    `json:"browser_session_id"`
	ItemID           string    `json:"item_id"`
	Asset            string    `json:"asset"`
	AppSessionID     string    `json:"app_session_id"`
	Version          uint64    `json:"version"`
	PurchasedAt      time.Time `json:"purchased_at"`
}

func (s *Store) UpsertStoreSession(ctx context.Context, session StoreSession) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO store_sessions (
			browser_session_id, asset, app_session_id, status, version, user_allocation, app_allocation, session_data, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(browser_session_id, asset) DO UPDATE SET
			app_session_id = excluded.app_session_id,
			status = excluded.status,
			version = excluded.version,
			user_allocation = excluded.user_allocation,
			app_allocation = excluded.app_allocation,
			session_data = excluded.session_data,
			updated_at = excluded.updated_at
	`, session.BrowserSessionID, session.Asset, session.AppSessionID, session.Status, session.Version, session.UserAllocation, session.AppAllocation, session.SessionData, formatTime(session.CreatedAt), formatTime(session.UpdatedAt))
	return err
}

func (s *Store) GetStoreSession(ctx context.Context, browserSessionID string, asset string) (*StoreSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT browser_session_id, asset, app_session_id, status, version, user_allocation, app_allocation, session_data, created_at, updated_at
		FROM store_sessions
		WHERE browser_session_id = ? AND asset = ?
	`, browserSessionID, asset)
	session, err := scanStoreSession(row)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *Store) RecordPurchase(ctx context.Context, purchase Purchase) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO purchases (id, browser_session_id, item_id, asset, app_session_id, version, purchased_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, purchase.ID, purchase.BrowserSessionID, purchase.ItemID, purchase.Asset, purchase.AppSessionID, purchase.Version, formatTime(purchase.PurchasedAt))
	if err != nil {
		if isSQLiteUniqueError(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

func (s *Store) HasPurchase(ctx context.Context, browserSessionID string, itemID string, asset string) (bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM purchases WHERE browser_session_id = ? AND item_id = ? AND asset = ? LIMIT 1
	`, browserSessionID, itemID, asset)
	var marker int
	if err := row.Scan(&marker); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) ListPurchasesByBrowser(ctx context.Context, browserSessionID string, asset string) ([]Purchase, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, browser_session_id, item_id, asset, app_session_id, version, purchased_at
		FROM purchases
		WHERE browser_session_id = ? AND asset = ?
		ORDER BY purchased_at DESC
	`, browserSessionID, asset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Purchase{}
	for rows.Next() {
		purchase, err := scanPurchase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *purchase)
	}
	return out, rows.Err()
}

func scanStoreSession(scanner interface{ Scan(dest ...any) error }) (*StoreSession, error) {
	var session StoreSession
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(
		&session.BrowserSessionID,
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
		return nil, fmt.Errorf("parse store session created_at: %w", err)
	}
	session.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse store session updated_at: %w", err)
	}
	return &session, nil
}

func scanPurchase(scanner interface{ Scan(dest ...any) error }) (*Purchase, error) {
	var purchase Purchase
	var purchasedAt string
	if err := scanner.Scan(
		&purchase.ID,
		&purchase.BrowserSessionID,
		&purchase.ItemID,
		&purchase.Asset,
		&purchase.AppSessionID,
		&purchase.Version,
		&purchasedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var err error
	purchase.PurchasedAt, err = parseTime(purchasedAt)
	if err != nil {
		return nil, fmt.Errorf("parse purchase purchased_at: %w", err)
	}
	return &purchase, nil
}

func isSQLiteUniqueError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}
