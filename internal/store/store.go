package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const operatorLeaseName = "operator"

var (
	ErrNotFound = errors.New("store: not found")
	ErrConflict = errors.New("store: conflict")
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

type PaymentRequest struct {
	ID          string
	Slug        string
	Title       string
	Description string
	Asset       string
	Amount      string
	Status      string
	OrderID     string
	OperationID string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Order struct {
	ID               string
	PaymentRequestID string
	OperationID      string
	AppSessionID     string
	Title            string
	Description      string
	Asset            string
	Amount           string
	Status           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Payout struct {
	ID                string
	OperationID       string
	Asset             string
	Amount            string
	DestinationWallet string
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Operation struct {
	ID           string
	Type         string
	ResourceID   string
	Status       string
	ErrorMessage string
	Payload      string
	QueuedAt     time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
}

type OperatorLease struct {
	LeaseName    string
	SessionToken string
	AcquiredAt   time.Time
	HeartbeatAt  time.Time
	ExpiresAt    time.Time
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
		`CREATE TABLE IF NOT EXISTS payment_requests (
			id TEXT PRIMARY KEY,
			slug TEXT NOT NULL UNIQUE,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			asset TEXT NOT NULL,
			amount TEXT NOT NULL,
			status TEXT NOT NULL,
			order_id TEXT NOT NULL DEFAULT '',
			operation_id TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS orders (
			id TEXT PRIMARY KEY,
			payment_request_id TEXT NOT NULL,
			operation_id TEXT NOT NULL,
			app_session_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			asset TEXT NOT NULL,
			amount TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS payouts (
			id TEXT PRIMARY KEY,
			operation_id TEXT NOT NULL,
			asset TEXT NOT NULL,
			amount TEXT NOT NULL,
			destination_wallet TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS operations (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			status TEXT NOT NULL,
			error_message TEXT NOT NULL DEFAULT '',
			payload TEXT NOT NULL DEFAULT '{}',
			queued_at DATETIME NOT NULL,
			started_at DATETIME,
			completed_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS operator_lease (
			lease_name TEXT PRIMARY KEY,
			session_token TEXT NOT NULL,
			acquired_at DATETIME NOT NULL,
			heartbeat_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS store_sessions (
			browser_session_id TEXT NOT NULL,
			asset TEXT NOT NULL,
			app_session_id TEXT NOT NULL,
			status TEXT NOT NULL,
			version INTEGER NOT NULL,
			user_allocation TEXT NOT NULL,
			app_allocation TEXT NOT NULL,
			session_data TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (browser_session_id, asset)
		);`,
		`CREATE TABLE IF NOT EXISTS purchases (
			id TEXT PRIMARY KEY,
			browser_session_id TEXT NOT NULL,
			item_id TEXT NOT NULL,
			asset TEXT NOT NULL,
			app_session_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			purchased_at DATETIME NOT NULL,
			UNIQUE(browser_session_id, item_id, asset)
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite migrate: %w", err)
		}
	}
	return nil
}

func (s *Store) ClearLease(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM operator_lease WHERE lease_name = ?`, operatorLeaseName)
	return err
}

func (s *Store) GetLease(ctx context.Context) (*OperatorLease, error) {
	row := s.db.QueryRowContext(ctx, `SELECT lease_name, session_token, acquired_at, heartbeat_at, expires_at FROM operator_lease WHERE lease_name = ?`, operatorLeaseName)
	lease, err := scanLease(row)
	if err != nil {
		return nil, err
	}
	return lease, nil
}

func (s *Store) AcquireLease(ctx context.Context, sessionToken string, ttl time.Duration) (*OperatorLease, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)

	current, err := scanLease(tx.QueryRowContext(ctx, `SELECT lease_name, session_token, acquired_at, heartbeat_at, expires_at FROM operator_lease WHERE lease_name = ?`, operatorLeaseName))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if current != nil && current.SessionToken != sessionToken && current.ExpiresAt.After(now) {
		return nil, ErrConflict
	}

	lease := &OperatorLease{
		LeaseName:    operatorLeaseName,
		SessionToken: sessionToken,
		AcquiredAt:   now,
		HeartbeatAt:  now,
		ExpiresAt:    now.Add(ttl),
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO operator_lease (lease_name, session_token, acquired_at, heartbeat_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(lease_name) DO UPDATE SET
			session_token = excluded.session_token,
			acquired_at = excluded.acquired_at,
			heartbeat_at = excluded.heartbeat_at,
			expires_at = excluded.expires_at
	`, lease.LeaseName, lease.SessionToken, formatTime(lease.AcquiredAt), formatTime(lease.HeartbeatAt), formatTime(lease.ExpiresAt))
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return lease, nil
}

func (s *Store) ReleaseLease(ctx context.Context, sessionToken string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM operator_lease WHERE lease_name = ? AND session_token = ?`, operatorLeaseName, sessionToken)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) HeartbeatLease(ctx context.Context, sessionToken string, ttl time.Duration) (*OperatorLease, error) {
	now := s.now().UTC()
	expiresAt := now.Add(ttl)
	result, err := s.db.ExecContext(ctx, `
		UPDATE operator_lease
		SET heartbeat_at = ?, expires_at = ?
		WHERE lease_name = ? AND session_token = ?
	`, formatTime(now), formatTime(expiresAt), operatorLeaseName, sessionToken)
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, ErrNotFound
	}
	return s.GetLease(ctx)
}

func (s *Store) ActiveLeaseForSession(ctx context.Context, sessionToken string) (*OperatorLease, error) {
	lease, err := s.GetLease(ctx)
	if err != nil {
		return nil, err
	}
	if lease.SessionToken != sessionToken || !lease.ExpiresAt.After(s.now().UTC()) {
		return nil, ErrNotFound
	}
	return lease, nil
}

func (s *Store) CreatePaymentRequest(ctx context.Context, pr PaymentRequest) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO payment_requests (id, slug, title, description, asset, amount, status, order_id, operation_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, pr.ID, pr.Slug, pr.Title, pr.Description, pr.Asset, pr.Amount, pr.Status, pr.OrderID, pr.OperationID, formatTime(pr.CreatedAt), formatTime(pr.UpdatedAt))
	return err
}

func (s *Store) GetPaymentRequestBySlug(ctx context.Context, slug string) (*PaymentRequest, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, slug, title, description, asset, amount, status, order_id, operation_id, created_at, updated_at
		FROM payment_requests
		WHERE slug = ?
	`, slug)
	return scanPaymentRequest(row)
}

func (s *Store) ListPaymentRequests(ctx context.Context, limit int) ([]PaymentRequest, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, slug, title, description, asset, amount, status, order_id, operation_id, created_at, updated_at
		FROM payment_requests
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectPaymentRequests(rows)
}

func (s *Store) BeginPaymentRequestProcessing(ctx context.Context, slug string, order Order, op Operation) (*PaymentRequest, *Order, *Operation, bool, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, nil, false, err
	}
	defer rollback(tx)

	pr, err := scanPaymentRequest(tx.QueryRowContext(ctx, `
		SELECT id, slug, title, description, asset, amount, status, order_id, operation_id, created_at, updated_at
		FROM payment_requests
		WHERE slug = ?
	`, slug))
	if err != nil {
		return nil, nil, nil, false, err
	}

	switch pr.Status {
	case "pending":
		order.PaymentRequestID = pr.ID
		order.Title = pr.Title
		order.Description = pr.Description
		order.Asset = pr.Asset
		order.Amount = pr.Amount
		pr.Status = "processing"
		pr.OrderID = order.ID
		pr.OperationID = op.ID
		pr.UpdatedAt = now

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO orders (id, payment_request_id, operation_id, app_session_id, title, description, asset, amount, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, order.ID, order.PaymentRequestID, order.OperationID, order.AppSessionID, order.Title, order.Description, order.Asset, order.Amount, order.Status, formatTime(order.CreatedAt), formatTime(order.UpdatedAt)); err != nil {
			return nil, nil, nil, false, err
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO operations (id, type, resource_id, status, error_message, payload, queued_at, started_at, completed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, op.ID, op.Type, op.ResourceID, op.Status, op.ErrorMessage, op.Payload, formatTime(op.QueuedAt), nullableTime(op.StartedAt), nullableTime(op.CompletedAt)); err != nil {
			return nil, nil, nil, false, err
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE payment_requests
			SET status = ?, order_id = ?, operation_id = ?, updated_at = ?
			WHERE id = ?
		`, pr.Status, pr.OrderID, pr.OperationID, formatTime(pr.UpdatedAt), pr.ID); err != nil {
			return nil, nil, nil, false, err
		}

		if err := tx.Commit(); err != nil {
			return nil, nil, nil, false, err
		}
		return pr, &order, &op, true, nil

	case "processing", "completed":
		var existingOrder *Order
		if pr.OrderID != "" {
			existingOrder, err = scanOrder(tx.QueryRowContext(ctx, `
				SELECT id, payment_request_id, operation_id, app_session_id, title, description, asset, amount, status, created_at, updated_at
				FROM orders WHERE id = ?
			`, pr.OrderID))
			if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, nil, nil, false, err
			}
		}
		var existingOp *Operation
		if pr.OperationID != "" {
			existingOp, err = scanOperation(tx.QueryRowContext(ctx, `
				SELECT id, type, resource_id, status, error_message, payload, queued_at, started_at, completed_at
				FROM operations WHERE id = ?
			`, pr.OperationID))
			if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, nil, nil, false, err
			}
		}
		if err := tx.Commit(); err != nil {
			return nil, nil, nil, false, err
		}
		return pr, existingOrder, existingOp, false, nil

	default:
		return nil, nil, nil, false, ErrConflict
	}
}

func (s *Store) UpdatePaymentRequestState(ctx context.Context, id string, status string, orderID string, operationID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE payment_requests
		SET status = ?, order_id = ?, operation_id = ?, updated_at = ?
		WHERE id = ?
	`, status, orderID, operationID, formatTime(s.now().UTC()), id)
	return err
}

func (s *Store) GetOrder(ctx context.Context, id string) (*Order, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, payment_request_id, operation_id, app_session_id, title, description, asset, amount, status, created_at, updated_at
		FROM orders WHERE id = ?
	`, id)
	return scanOrder(row)
}

func (s *Store) ListOrders(ctx context.Context, limit int) ([]Order, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, payment_request_id, operation_id, app_session_id, title, description, asset, amount, status, created_at, updated_at
		FROM orders
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectOrders(rows)
}

func (s *Store) UpdateOrder(ctx context.Context, order Order) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE orders
		SET operation_id = ?, app_session_id = ?, status = ?, updated_at = ?
		WHERE id = ?
	`, order.OperationID, order.AppSessionID, order.Status, formatTime(order.UpdatedAt), order.ID)
	return err
}

func (s *Store) CreatePayoutWithOperation(ctx context.Context, payout Payout, op Operation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO payouts (id, operation_id, asset, amount, destination_wallet, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, payout.ID, payout.OperationID, payout.Asset, payout.Amount, payout.DestinationWallet, payout.Status, formatTime(payout.CreatedAt), formatTime(payout.UpdatedAt)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO operations (id, type, resource_id, status, error_message, payload, queued_at, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, op.ID, op.Type, op.ResourceID, op.Status, op.ErrorMessage, op.Payload, formatTime(op.QueuedAt), nullableTime(op.StartedAt), nullableTime(op.CompletedAt)); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) GetPayout(ctx context.Context, id string) (*Payout, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, operation_id, asset, amount, destination_wallet, status, created_at, updated_at
		FROM payouts WHERE id = ?
	`, id)
	return scanPayout(row)
}

func (s *Store) ListPayouts(ctx context.Context, limit int) ([]Payout, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, operation_id, asset, amount, destination_wallet, status, created_at, updated_at
		FROM payouts
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectPayouts(rows)
}

func (s *Store) UpdatePayout(ctx context.Context, payout Payout) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE payouts
		SET operation_id = ?, status = ?, updated_at = ?
		WHERE id = ?
	`, payout.OperationID, payout.Status, formatTime(payout.UpdatedAt), payout.ID)
	return err
}

func (s *Store) CreateOperation(ctx context.Context, op Operation) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO operations (id, type, resource_id, status, error_message, payload, queued_at, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, op.ID, op.Type, op.ResourceID, op.Status, op.ErrorMessage, op.Payload, formatTime(op.QueuedAt), nullableTime(op.StartedAt), nullableTime(op.CompletedAt))
	return err
}

func (s *Store) GetOperation(ctx context.Context, id string) (*Operation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, type, resource_id, status, error_message, payload, queued_at, started_at, completed_at
		FROM operations WHERE id = ?
	`, id)
	return scanOperation(row)
}

func (s *Store) ListOperations(ctx context.Context, limit int) ([]Operation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, resource_id, status, error_message, payload, queued_at, started_at, completed_at
		FROM operations
		ORDER BY queued_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectOperations(rows)
}

func (s *Store) ListOperationsByStatuses(ctx context.Context, limit int, statuses ...string) ([]Operation, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(statuses)), ",")
	args := make([]any, 0, len(statuses)+1)
	for _, status := range statuses {
		args = append(args, status)
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, type, resource_id, status, error_message, payload, queued_at, started_at, completed_at
		FROM operations
		WHERE status IN (%s)
		ORDER BY queued_at ASC
		LIMIT ?
	`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectOperations(rows)
}

func (s *Store) UpdateOperation(ctx context.Context, op Operation) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE operations
		SET status = ?, error_message = ?, payload = ?, started_at = ?, completed_at = ?
		WHERE id = ?
	`, op.Status, op.ErrorMessage, op.Payload, nullableTime(op.StartedAt), nullableTime(op.CompletedAt), op.ID)
	return err
}

func (s *Store) MarkRunningOperationsInterrupted(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE operations
		SET status = 'failed', error_message = 'interrupted by server restart', completed_at = ?
		WHERE status = 'running'
	`, formatTime(s.now().UTC()))
	return err
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, raw)
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func scanPaymentRequest(scanner interface{ Scan(...any) error }) (*PaymentRequest, error) {
	var pr PaymentRequest
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(&pr.ID, &pr.Slug, &pr.Title, &pr.Description, &pr.Asset, &pr.Amount, &pr.Status, &pr.OrderID, &pr.OperationID, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var err error
	if pr.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if pr.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &pr, nil
}

func collectPaymentRequests(rows *sql.Rows) ([]PaymentRequest, error) {
	var out []PaymentRequest
	for rows.Next() {
		pr, err := scanPaymentRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *pr)
	}
	return out, rows.Err()
}

func scanOrder(scanner interface{ Scan(...any) error }) (*Order, error) {
	var order Order
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(&order.ID, &order.PaymentRequestID, &order.OperationID, &order.AppSessionID, &order.Title, &order.Description, &order.Asset, &order.Amount, &order.Status, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var err error
	if order.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if order.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &order, nil
}

func collectOrders(rows *sql.Rows) ([]Order, error) {
	var out []Order
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *order)
	}
	return out, rows.Err()
}

func scanPayout(scanner interface{ Scan(...any) error }) (*Payout, error) {
	var payout Payout
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(&payout.ID, &payout.OperationID, &payout.Asset, &payout.Amount, &payout.DestinationWallet, &payout.Status, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var err error
	if payout.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if payout.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &payout, nil
}

func collectPayouts(rows *sql.Rows) ([]Payout, error) {
	var out []Payout
	for rows.Next() {
		payout, err := scanPayout(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *payout)
	}
	return out, rows.Err()
}

func scanOperation(scanner interface{ Scan(...any) error }) (*Operation, error) {
	var op Operation
	var queuedAt string
	var startedAt sql.NullString
	var completedAt sql.NullString
	if err := scanner.Scan(&op.ID, &op.Type, &op.ResourceID, &op.Status, &op.ErrorMessage, &op.Payload, &queuedAt, &startedAt, &completedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var err error
	if op.QueuedAt, err = parseTime(queuedAt); err != nil {
		return nil, err
	}
	if startedAt.Valid {
		parsed, err := parseTime(startedAt.String)
		if err != nil {
			return nil, err
		}
		op.StartedAt = &parsed
	}
	if completedAt.Valid {
		parsed, err := parseTime(completedAt.String)
		if err != nil {
			return nil, err
		}
		op.CompletedAt = &parsed
	}
	return &op, nil
}

func collectOperations(rows *sql.Rows) ([]Operation, error) {
	var out []Operation
	for rows.Next() {
		op, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *op)
	}
	return out, rows.Err()
}

func scanLease(scanner interface{ Scan(...any) error }) (*OperatorLease, error) {
	var lease OperatorLease
	var acquiredAt string
	var heartbeatAt string
	var expiresAt string
	if err := scanner.Scan(&lease.LeaseName, &lease.SessionToken, &acquiredAt, &heartbeatAt, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var err error
	if lease.AcquiredAt, err = parseTime(acquiredAt); err != nil {
		return nil, err
	}
	if lease.HeartbeatAt, err = parseTime(heartbeatAt); err != nil {
		return nil, err
	}
	if lease.ExpiresAt, err = parseTime(expiresAt); err != nil {
		return nil, err
	}
	return &lease, nil
}
