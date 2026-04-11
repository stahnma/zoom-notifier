package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/stahnma/zoom-notifier/internal/store"
)

// escapeLike escapes LIKE metacharacters so the pattern is treated as a literal substring.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

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
	defer closeRows(rows)

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
	filter, hasFilters, err := s.CheckFilter(ctx, tenantID, topic)
	if err != nil {
		return false, err
	}
	if !hasFilters {
		return true, nil
	}
	return filter != nil, nil
}

func (s *SQLiteStore) GetMatchingFilter(ctx context.Context, tenantID string, topic string) (*store.MeetingFilter, error) {
	filter, _, err := s.CheckFilter(ctx, tenantID, topic)
	return filter, err
}

// CheckFilter performs a single-pass filter check: returns the matching filter (if any),
// whether any filters exist for this tenant, and any error. When hasFilters is false,
// all meetings are allowed (no filters configured).
func (s *SQLiteStore) CheckFilter(ctx context.Context, tenantID string, topic string) (filter *store.MeetingFilter, hasFilters bool, err error) {
	// Count total filters for this tenant
	var count int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM meeting_filters WHERE tenant_id = ?`, tenantID,
	).Scan(&count)
	if err != nil {
		return nil, false, fmt.Errorf("count filters: %w", err)
	}
	if count == 0 {
		return nil, false, nil
	}

	// Return the first matching filter (case-insensitive substring)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, pattern, msg_suffix, include_link
		 FROM meeting_filters WHERE tenant_id = ? AND LOWER(?) LIKE '%' || LOWER(pattern) || '%' ESCAPE '\'
		 ORDER BY id LIMIT 1`,
		tenantID, escapeLike(topic),
	)
	f := &store.MeetingFilter{}
	err = row.Scan(&f.ID, &f.TenantID, &f.Pattern, &f.MsgSuffix, &f.IncludeLink)
	if err == sql.ErrNoRows {
		return nil, true, nil
	}
	if err != nil {
		return nil, true, fmt.Errorf("get matching filter: %w", err)
	}
	return f, true, nil
}
