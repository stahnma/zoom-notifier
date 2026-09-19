package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func (s *SQLiteStore) UpsertZoomCredentials(ctx context.Context, z *store.ZoomCredentials) error {
	clientSecret, err := s.cipher.Seal(z.ClientSecret)
	if err != nil {
		return fmt.Errorf("encrypt zoom client secret: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO zoom_credentials (tenant_id, client_id, client_secret, account_id)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(tenant_id) DO UPDATE SET
		   client_id = excluded.client_id,
		   client_secret = excluded.client_secret,
		   account_id = excluded.account_id`,
		z.TenantID, z.ClientID, clientSecret, z.AccountID,
	)
	if err != nil {
		return fmt.Errorf("upsert zoom credentials: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetZoomCredentials(ctx context.Context, tenantID string) (*store.ZoomCredentials, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT tenant_id, client_id, client_secret, account_id
		 FROM zoom_credentials WHERE tenant_id = ?`, tenantID,
	)
	z := &store.ZoomCredentials{}
	err := row.Scan(&z.TenantID, &z.ClientID, &z.ClientSecret, &z.AccountID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get zoom credentials: %w", err)
	}
	if z.ClientSecret, err = s.cipher.Open(z.ClientSecret); err != nil {
		return nil, fmt.Errorf("decrypt zoom client secret for tenant %s: %w", tenantID, err)
	}
	return z, nil
}
