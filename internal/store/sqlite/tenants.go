package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func (s *SQLiteStore) CreateTenant(ctx context.Context, t *store.Tenant) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tenants (id, team_name, bot_token, api_key, zoom_account_id, default_msg_suffix, default_include_link)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.TeamName, t.BotToken, t.APIKey, t.ZoomAccountID, t.DefaultMsgSuffix, t.DefaultIncludeLink,
	)
	if err != nil {
		return fmt.Errorf("create tenant: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetTenant(ctx context.Context, id string) (*store.Tenant, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, team_name, bot_token, api_key, installed_at, zoom_account_id, default_msg_suffix, default_include_link
		 FROM tenants WHERE id = ?`, id,
	)
	t := &store.Tenant{}
	err := row.Scan(&t.ID, &t.TeamName, &t.BotToken, &t.APIKey, &t.InstalledAt, &t.ZoomAccountID, &t.DefaultMsgSuffix, &t.DefaultIncludeLink)
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
		`SELECT id, team_name, bot_token, api_key, installed_at, zoom_account_id, default_msg_suffix, default_include_link
		 FROM tenants WHERE zoom_account_id = ?`, zoomAccountID,
	)
	if err != nil {
		return nil, fmt.Errorf("get tenants by zoom account: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tenants []*store.Tenant
	for rows.Next() {
		t := &store.Tenant{}
		if err := rows.Scan(&t.ID, &t.TeamName, &t.BotToken, &t.APIKey, &t.InstalledAt, &t.ZoomAccountID, &t.DefaultMsgSuffix, &t.DefaultIncludeLink); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (s *SQLiteStore) ListTenants(ctx context.Context) ([]*store.Tenant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, team_name, bot_token, api_key, installed_at, zoom_account_id, default_msg_suffix, default_include_link
		 FROM tenants`,
	)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tenants []*store.Tenant
	for rows.Next() {
		t := &store.Tenant{}
		if err := rows.Scan(&t.ID, &t.TeamName, &t.BotToken, &t.APIKey, &t.InstalledAt, &t.ZoomAccountID, &t.DefaultMsgSuffix, &t.DefaultIncludeLink); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (s *SQLiteStore) UpdateTenant(ctx context.Context, t *store.Tenant) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tenants SET team_name = ?, bot_token = ?, zoom_account_id = ?, default_msg_suffix = ?, default_include_link = ? WHERE id = ?`,
		t.TeamName, t.BotToken, t.ZoomAccountID, t.DefaultMsgSuffix, t.DefaultIncludeLink, t.ID,
	)
	if err != nil {
		return fmt.Errorf("update tenant: %w", err)
	}
	return nil
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

func (s *SQLiteStore) UpdateTenantDefaults(ctx context.Context, tenantID string, suffix string, includeLink bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tenants SET default_msg_suffix = ?, default_include_link = ? WHERE id = ?`,
		suffix, includeLink, tenantID,
	)
	if err != nil {
		return fmt.Errorf("update tenant defaults: %w", err)
	}
	return nil
}
