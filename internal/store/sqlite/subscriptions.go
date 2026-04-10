package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func (s *SQLiteStore) CreateSubscription(ctx context.Context, sub *store.Subscription) error {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO subscriptions (tenant_id, type, meeting_id, target, enabled)
		 VALUES (?, ?, ?, ?, ?)`,
		sub.TenantID, sub.Type, sub.MeetingID, sub.Target, sub.Enabled,
	)
	if err != nil {
		return fmt.Errorf("create subscription: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	sub.ID = id
	return nil
}

func (s *SQLiteStore) GetSubscription(ctx context.Context, id int64) (*store.Subscription, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, type, meeting_id, target, enabled, created_at
		 FROM subscriptions WHERE id = ?`, id,
	)
	sub := &store.Subscription{}
	err := row.Scan(&sub.ID, &sub.TenantID, &sub.Type, &sub.MeetingID, &sub.Target,
		&sub.Enabled, &sub.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	return sub, nil
}

func (s *SQLiteStore) ListSubscriptions(ctx context.Context, tenantID string) ([]*store.Subscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, type, meeting_id, target, enabled, created_at
		 FROM subscriptions WHERE tenant_id = ?`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer closeRows(rows)

	var subs []*store.Subscription
	for rows.Next() {
		sub := &store.Subscription{}
		if err := rows.Scan(&sub.ID, &sub.TenantID, &sub.Type, &sub.MeetingID, &sub.Target,
			&sub.Enabled, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (s *SQLiteStore) GetSubscriptionsForMeeting(ctx context.Context, tenantID string, meetingID string) ([]*store.Subscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, type, meeting_id, target, enabled, created_at
		 FROM subscriptions
		 WHERE tenant_id = ? AND enabled = 1
		 AND (meeting_id = ? OR meeting_id IS NULL)`, tenantID, meetingID,
	)
	if err != nil {
		return nil, fmt.Errorf("get subscriptions for meeting: %w", err)
	}
	defer closeRows(rows)

	var subs []*store.Subscription
	for rows.Next() {
		sub := &store.Subscription{}
		if err := rows.Scan(&sub.ID, &sub.TenantID, &sub.Type, &sub.MeetingID, &sub.Target,
			&sub.Enabled, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (s *SQLiteStore) UpdateSubscription(ctx context.Context, sub *store.Subscription) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE subscriptions SET type = ?, meeting_id = ?, target = ?, enabled = ?
		 WHERE id = ?`,
		sub.Type, sub.MeetingID, sub.Target, sub.Enabled, sub.ID,
	)
	if err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteSubscription(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	return nil
}
