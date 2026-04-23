package store

import (
	"context"
	"fmt"
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
	PurchasedAt   time.Time `json:"purchased_at"`
}

func (s *Store) UpsertWalletSession(ctx context.Context, session WalletStoreSession) error {
	return s.UpsertStoreSession(ctx, StoreSession{
		BrowserSessionID: session.WalletAddress,
		Asset:            session.Asset,
		AppSessionID:     session.AppSessionID,
		Status:           session.Status,
		Version:          session.Version,
		UserAllocation:   session.UserAllocation,
		AppAllocation:    session.AppAllocation,
		SessionData:      session.SessionData,
		CreatedAt:        session.CreatedAt,
		UpdatedAt:        session.UpdatedAt,
	})
}

func (s *Store) GetWalletSession(ctx context.Context, walletAddress string, asset string) (*WalletStoreSession, error) {
	record, err := s.GetStoreSession(ctx, walletAddress, asset)
	if err != nil {
		return nil, err
	}
	return &WalletStoreSession{
		WalletAddress:  record.BrowserSessionID,
		Asset:          record.Asset,
		AppSessionID:   record.AppSessionID,
		Status:         record.Status,
		Version:        record.Version,
		UserAllocation: record.UserAllocation,
		AppAllocation:  record.AppAllocation,
		SessionData:    record.SessionData,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}, nil
}

func (s *Store) RecordWalletPurchase(ctx context.Context, purchase WalletPurchase) error {
	return s.RecordPurchase(ctx, Purchase{
		ID:               purchase.ID,
		BrowserSessionID: purchase.WalletAddress,
		ItemID:           purchase.ItemID,
		Asset:            purchase.Asset,
		AppSessionID:     purchase.AppSessionID,
		Version:          purchase.Version,
		PurchasedAt:      purchase.PurchasedAt,
	})
}

func (s *Store) HasWalletPurchase(ctx context.Context, walletAddress string, itemID string, asset string) (bool, error) {
	return s.HasPurchase(ctx, walletAddress, itemID, asset)
}

func (s *Store) ListPurchasesByWallet(ctx context.Context, walletAddress string, asset string) ([]WalletPurchase, error) {
	purchases, err := s.ListPurchasesByBrowser(ctx, walletAddress, asset)
	if err != nil {
		return nil, err
	}

	out := make([]WalletPurchase, 0, len(purchases))
	for _, purchase := range purchases {
		out = append(out, WalletPurchase{
			ID:            purchase.ID,
			WalletAddress: purchase.BrowserSessionID,
			ItemID:        purchase.ItemID,
			Asset:         purchase.Asset,
			AppSessionID:  purchase.AppSessionID,
			Version:       purchase.Version,
			PurchasedAt:   purchase.PurchasedAt,
		})
	}
	return out, nil
}

func WalletPurchaseID(walletAddress string, asset string, itemID string) string {
	return fmt.Sprintf("%s:%s:%s", walletAddress, asset, itemID)
}
