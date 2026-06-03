package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("store: not found")
	ErrConflict = errors.New("store: conflict")
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func New(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db, now: time.Now}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`PRAGMA journal_mode = WAL;`,
		`CREATE TABLE IF NOT EXISTS wallet_store_sessions (
			wallet_address TEXT NOT NULL,
			asset TEXT NOT NULL,
			app_session_id TEXT NOT NULL,
			status TEXT NOT NULL,
			version INTEGER NOT NULL,
			user_allocation TEXT NOT NULL,
			app_allocation TEXT NOT NULL,
			session_data TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (wallet_address, asset),
			UNIQUE(app_session_id, asset)
		);`,
		`CREATE TABLE IF NOT EXISTS wallet_purchases (
			id TEXT PRIMARY KEY,
			wallet_address TEXT NOT NULL,
			item_id TEXT NOT NULL,
			asset TEXT NOT NULL,
			app_session_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'submitted',
			session_data TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT '1970-01-01T00:00:00Z',
			updated_at DATETIME NOT NULL DEFAULT '1970-01-01T00:00:00Z',
			purchased_at DATETIME NOT NULL,
			UNIQUE(wallet_address, item_id, asset)
		);`,
		`CREATE TABLE IF NOT EXISTS wallet_deposit_checkpoints (
			id TEXT PRIMARY KEY,
			wallet_address TEXT NOT NULL,
			asset TEXT NOT NULL,
			app_session_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			amount TEXT NOT NULL,
			status TEXT NOT NULL,
			app_state_update TEXT NOT NULL,
			user_signature TEXT NOT NULL,
			app_signature TEXT NOT NULL,
			session_data TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(wallet_address, asset)
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite migrate: %w", err)
		}
	}

	columnDefaults := map[string]string{
		"status":       "TEXT NOT NULL DEFAULT 'submitted'",
		"session_data": "TEXT NOT NULL DEFAULT ''",
		"created_at":   "DATETIME NOT NULL DEFAULT '1970-01-01T00:00:00Z'",
		"updated_at":   "DATETIME NOT NULL DEFAULT '1970-01-01T00:00:00Z'",
	}
	for column, definition := range columnDefaults {
		if err := s.ensureColumn(ctx, "wallet_purchases", column, definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureColumn(ctx context.Context, table string, column string, definition string) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return fmt.Errorf("sqlite inspect %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("sqlite scan %s columns: %w", table, err)
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite inspect %s: %w", table, err)
	}

	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)); err != nil {
		return fmt.Errorf("sqlite add %s.%s: %w", table, column, err)
	}
	return nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, raw)
}
