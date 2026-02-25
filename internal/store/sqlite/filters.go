package sqlite

import (
	"context"
	"fmt"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

func (s *SQLiteStore) CreateFilter(ctx context.Context, f *store.MeetingFilter) error {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO meeting_filters (tenant_id, pattern) VALUES (?, ?)`,
		f.TenantID, f.Pattern,
	)
	if err != nil {
		return fmt.Errorf("create filter: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	f.ID = id
	return nil
}

func (s *SQLiteStore) DeleteFilter(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM meeting_filters WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete filter: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListFilters(ctx context.Context, tenantID string) ([]*store.MeetingFilter, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, pattern FROM meeting_filters WHERE tenant_id = ?`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list filters: %w", err)
	}
	defer rows.Close()

	var filters []*store.MeetingFilter
	for rows.Next() {
		f := &store.MeetingFilter{}
		if err := rows.Scan(&f.ID, &f.TenantID, &f.Pattern); err != nil {
			return nil, fmt.Errorf("scan filter: %w", err)
		}
		filters = append(filters, f)
	}
	return filters, rows.Err()
}

func (s *SQLiteStore) MatchesFilter(ctx context.Context, tenantID string, topic string) (bool, error) {
	// If no filters exist for this tenant, allow all (return true)
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM meeting_filters WHERE tenant_id = ?`, tenantID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count filters: %w", err)
	}
	if count == 0 {
		return true, nil
	}

	// Check if topic matches any filter
	var matchCount int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM meeting_filters WHERE tenant_id = ? AND pattern = ?`,
		tenantID, topic,
	).Scan(&matchCount)
	if err != nil {
		return false, fmt.Errorf("match filter: %w", err)
	}
	return matchCount > 0, nil
}
