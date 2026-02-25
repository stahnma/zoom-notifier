package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

func (s *SQLiteStore) UpsertZoomCredentials(ctx context.Context, z *store.ZoomCredentials) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO zoom_credentials (tenant_id, client_id, client_secret, account_id)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(tenant_id) DO UPDATE SET
		   client_id = excluded.client_id,
		   client_secret = excluded.client_secret,
		   account_id = excluded.account_id`,
		z.TenantID, z.ClientID, z.ClientSecret, z.AccountID,
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
	return z, nil
}
