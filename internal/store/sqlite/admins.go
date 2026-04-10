package sqlite

import (
	"context"
	"fmt"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func (s *SQLiteStore) AddAdmin(ctx context.Context, tenantID string, slackUserID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO tenant_admins (tenant_id, slack_user_id) VALUES (?, ?)`,
		tenantID, slackUserID,
	)
	if err != nil {
		return fmt.Errorf("add admin: %w", err)
	}
	return nil
}

func (s *SQLiteStore) RemoveAdmin(ctx context.Context, tenantID string, slackUserID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM tenant_admins WHERE tenant_id = ? AND slack_user_id = ?`,
		tenantID, slackUserID,
	)
	if err != nil {
		return fmt.Errorf("remove admin: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListAdmins(ctx context.Context, tenantID string) ([]*store.TenantAdmin, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tenant_id, slack_user_id FROM tenant_admins WHERE tenant_id = ?`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list admins: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var admins []*store.TenantAdmin
	for rows.Next() {
		a := &store.TenantAdmin{}
		if err := rows.Scan(&a.TenantID, &a.SlackUserID); err != nil {
			return nil, fmt.Errorf("scan admin: %w", err)
		}
		admins = append(admins, a)
	}
	return admins, rows.Err()
}

func (s *SQLiteStore) IsAdmin(ctx context.Context, tenantID string, slackUserID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tenant_admins WHERE tenant_id = ? AND slack_user_id = ?`,
		tenantID, slackUserID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("is admin: %w", err)
	}
	return count > 0, nil
}
