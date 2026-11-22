package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/stahnma/zoom-notifier/internal/store"
)

// UpsertZoomAccount writes every field of a. Callers wanting merge semantics
// should read the existing account first.
func (s *SQLiteStore) UpsertZoomAccount(ctx context.Context, a *store.ZoomAccount) error {
	webhookSecret, err := s.cipher.Seal(a.WebhookSecret)
	if err != nil {
		return fmt.Errorf("encrypt zoom webhook secret: %w", err)
	}
	clientSecret, err := s.cipher.Seal(a.ClientSecret)
	if err != nil {
		return fmt.Errorf("encrypt zoom client secret: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO zoom_accounts (account_id, webhook_secret, client_id, client_secret)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(account_id) DO UPDATE SET
		   webhook_secret = excluded.webhook_secret,
		   client_id = excluded.client_id,
		   client_secret = excluded.client_secret`,
		a.AccountID, webhookSecret, a.ClientID, clientSecret,
	)
	if err != nil {
		return fmt.Errorf("upsert zoom account: %w", err)
	}
	return nil
}

func (s *SQLiteStore) EnsureZoomAccount(ctx context.Context, accountID string) error {
	if accountID == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO zoom_accounts (account_id) VALUES (?)`, accountID,
	); err != nil {
		return fmt.Errorf("ensure zoom account: %w", err)
	}
	return nil
}

const zoomAccountColumns = `account_id, webhook_secret, client_id, client_secret, created_at`

func (s *SQLiteStore) scanZoomAccount(scan func(dest ...any) error) (*store.ZoomAccount, error) {
	a := &store.ZoomAccount{}
	if err := scan(&a.AccountID, &a.WebhookSecret, &a.ClientID, &a.ClientSecret, &a.CreatedAt); err != nil {
		return nil, err
	}
	var err error
	if a.WebhookSecret, err = s.cipher.Open(a.WebhookSecret); err != nil {
		return nil, fmt.Errorf("decrypt zoom webhook secret for account %s: %w", a.AccountID, err)
	}
	if a.ClientSecret, err = s.cipher.Open(a.ClientSecret); err != nil {
		return nil, fmt.Errorf("decrypt zoom client secret for account %s: %w", a.AccountID, err)
	}
	return a, nil
}

func (s *SQLiteStore) GetZoomAccount(ctx context.Context, accountID string) (*store.ZoomAccount, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+zoomAccountColumns+` FROM zoom_accounts WHERE account_id = ?`, accountID)
	a, err := s.scanZoomAccount(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get zoom account: %w", err)
	}
	return a, nil
}

func (s *SQLiteStore) ListZoomAccounts(ctx context.Context) ([]*store.ZoomAccount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+zoomAccountColumns+` FROM zoom_accounts ORDER BY account_id`)
	if err != nil {
		return nil, fmt.Errorf("list zoom accounts: %w", err)
	}
	defer closeRows(rows)

	var accounts []*store.ZoomAccount
	for rows.Next() {
		a, err := s.scanZoomAccount(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan zoom account: %w", err)
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// DeleteZoomAccount removes the account and, by cascade, its meeting state.
// Tenants linked to it keep their zoom_account_id; callers should unlink first.
func (s *SQLiteStore) DeleteZoomAccount(ctx context.Context, accountID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM zoom_accounts WHERE account_id = ?`, accountID); err != nil {
		return fmt.Errorf("delete zoom account: %w", err)
	}
	return nil
}
