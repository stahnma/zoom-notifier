package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func (s *SQLiteStore) CreateFilter(ctx context.Context, f *store.MeetingFilter) error {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO meeting_filters (tenant_id, pattern, msg_suffix, include_link) VALUES (?, ?, ?, ?)`,
		f.TenantID, f.Pattern, f.MsgSuffix, f.IncludeLink,
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

func (s *SQLiteStore) UpdateFilter(ctx context.Context, f *store.MeetingFilter) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE meeting_filters SET pattern = ?, msg_suffix = ?, include_link = ? WHERE id = ?`,
		f.Pattern, f.MsgSuffix, f.IncludeLink, f.ID,
	)
	if err != nil {
		return fmt.Errorf("update filter: %w", err)
	}
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
		`SELECT id, tenant_id, pattern, msg_suffix, include_link FROM meeting_filters WHERE tenant_id = ?`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list filters: %w", err)
	}
	defer rows.Close()

	var filters []*store.MeetingFilter
	for rows.Next() {
		f := &store.MeetingFilter{}
		if err := rows.Scan(&f.ID, &f.TenantID, &f.Pattern, &f.MsgSuffix, &f.IncludeLink); err != nil {
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

	// Check if topic matches any filter (case-insensitive substring)
	var matchCount int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM meeting_filters WHERE tenant_id = ? AND LOWER(?) LIKE '%' || LOWER(pattern) || '%'`,
		tenantID, topic,
	).Scan(&matchCount)
	if err != nil {
		return false, fmt.Errorf("match filter: %w", err)
	}
	return matchCount > 0, nil
}

func (s *SQLiteStore) GetMatchingFilter(ctx context.Context, tenantID string, topic string) (*store.MeetingFilter, error) {
	// If no filters exist for this tenant, return nil (no filter applies)
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM meeting_filters WHERE tenant_id = ?`, tenantID,
	).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("count filters: %w", err)
	}
	if count == 0 {
		return nil, nil
	}

	// Return the first matching filter (case-insensitive substring)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, pattern, msg_suffix, include_link
		 FROM meeting_filters WHERE tenant_id = ? AND LOWER(?) LIKE '%' || LOWER(pattern) || '%'
		 ORDER BY id LIMIT 1`,
		tenantID, topic,
	)
	f := &store.MeetingFilter{}
	err = row.Scan(&f.ID, &f.TenantID, &f.Pattern, &f.MsgSuffix, &f.IncludeLink)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get matching filter: %w", err)
	}
	return f, nil
}
