package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func (s *SQLiteStore) UpsertMeeting(ctx context.Context, m *store.ActiveMeeting) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO active_meetings (meeting_id, tenant_id, topic, host_id, start_time, join_url)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(meeting_id) DO UPDATE SET
		   topic = excluded.topic,
		   host_id = excluded.host_id,
		   start_time = excluded.start_time,
		   join_url = excluded.join_url`,
		m.MeetingID, m.TenantID, m.Topic, m.HostID, m.StartTime, m.JoinURL,
	)
	if err != nil {
		return fmt.Errorf("upsert meeting: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetMeeting(ctx context.Context, meetingID string) (*store.ActiveMeeting, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT meeting_id, tenant_id, topic, host_id, start_time, join_url
		 FROM active_meetings WHERE meeting_id = ?`, meetingID,
	)
	m := &store.ActiveMeeting{}
	err := row.Scan(&m.MeetingID, &m.TenantID, &m.Topic, &m.HostID, &m.StartTime, &m.JoinURL)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get meeting: %w", err)
	}
	return m, nil
}

func (s *SQLiteStore) ListActiveMeetings(ctx context.Context, tenantID string) ([]*store.ActiveMeeting, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT meeting_id, tenant_id, topic, host_id, start_time, join_url
		 FROM active_meetings WHERE tenant_id = ?`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list active meetings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var meetings []*store.ActiveMeeting
	for rows.Next() {
		m := &store.ActiveMeeting{}
		if err := rows.Scan(&m.MeetingID, &m.TenantID, &m.Topic, &m.HostID, &m.StartTime, &m.JoinURL); err != nil {
			return nil, fmt.Errorf("scan meeting: %w", err)
		}
		meetings = append(meetings, m)
	}
	return meetings, rows.Err()
}

func (s *SQLiteStore) DeleteMeeting(ctx context.Context, meetingID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM active_meetings WHERE meeting_id = ?`, meetingID)
	if err != nil {
		return fmt.Errorf("delete meeting: %w", err)
	}
	return nil
}

func (s *SQLiteStore) AddParticipant(ctx context.Context, p *store.Participant) error {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO participants (meeting_id, user_name, email, join_time)
		 VALUES (?, ?, ?, ?)`,
		p.MeetingID, p.UserName, p.Email, p.JoinTime,
	)
	if err != nil {
		return fmt.Errorf("add participant: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	p.ID = id
	return nil
}

func (s *SQLiteStore) SetParticipantLeft(ctx context.Context, meetingID string, userName string, leaveTime *time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE participants SET leave_time = ?
		 WHERE meeting_id = ? AND user_name = ? AND leave_time IS NULL`,
		leaveTime, meetingID, userName,
	)
	if err != nil {
		return fmt.Errorf("set participant left: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetActiveParticipants(ctx context.Context, meetingID string) ([]*store.Participant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, meeting_id, user_name, email, join_time, leave_time
		 FROM participants
		 WHERE meeting_id = ? AND leave_time IS NULL`, meetingID,
	)
	if err != nil {
		return nil, fmt.Errorf("get active participants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var participants []*store.Participant
	for rows.Next() {
		p := &store.Participant{}
		if err := rows.Scan(&p.ID, &p.MeetingID, &p.UserName, &p.Email, &p.JoinTime, &p.LeaveTime); err != nil {
			return nil, fmt.Errorf("scan participant: %w", err)
		}
		participants = append(participants, p)
	}
	return participants, rows.Err()
}

func (s *SQLiteStore) DeleteParticipantsForMeeting(ctx context.Context, meetingID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM participants WHERE meeting_id = ?`, meetingID)
	if err != nil {
		return fmt.Errorf("delete participants: %w", err)
	}
	return nil
}
