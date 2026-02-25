package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

func (s *SQLiteStore) CreateTenant(ctx context.Context, t *store.Tenant) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tenants (id, team_name, bot_token, api_key, zoom_account_id)
		 VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.TeamName, t.BotToken, t.APIKey, t.ZoomAccountID,
	)
	if err != nil {
		return fmt.Errorf("create tenant: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetTenant(ctx context.Context, id string) (*store.Tenant, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, team_name, bot_token, api_key, installed_at, zoom_account_id
		 FROM tenants WHERE id = ?`, id,
	)
	t := &store.Tenant{}
	err := row.Scan(&t.ID, &t.TeamName, &t.BotToken, &t.APIKey, &t.InstalledAt, &t.ZoomAccountID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	return t, nil
}

func (s *SQLiteStore) GetTenantByZoomAccount(ctx context.Context, zoomAccountID string) ([]*store.Tenant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, team_name, bot_token, api_key, installed_at, zoom_account_id
		 FROM tenants WHERE zoom_account_id = ?`, zoomAccountID,
	)
	if err != nil {
		return nil, fmt.Errorf("get tenants by zoom account: %w", err)
	}
	defer rows.Close()

	var tenants []*store.Tenant
	for rows.Next() {
		t := &store.Tenant{}
		if err := rows.Scan(&t.ID, &t.TeamName, &t.BotToken, &t.APIKey, &t.InstalledAt, &t.ZoomAccountID); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (s *SQLiteStore) ListTenants(ctx context.Context) ([]*store.Tenant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, team_name, bot_token, api_key, installed_at, zoom_account_id
		 FROM tenants`,
	)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*store.Tenant
	for rows.Next() {
		t := &store.Tenant{}
		if err := rows.Scan(&t.ID, &t.TeamName, &t.BotToken, &t.APIKey, &t.InstalledAt, &t.ZoomAccountID); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (s *SQLiteStore) DeleteTenant(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tenants WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateTenantAPIKey(ctx context.Context, id string, newKey string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE tenants SET api_key = ? WHERE id = ?`, newKey, id)
	if err != nil {
		return fmt.Errorf("update tenant api key: %w", err)
	}
	return nil
}
