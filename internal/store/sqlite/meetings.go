package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func (s *SQLiteStore) UpsertMeeting(ctx context.Context, m *store.ActiveMeeting) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO active_meetings (zoom_account_id, meeting_id, topic, host_id, start_time, join_url)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(zoom_account_id, meeting_id) DO UPDATE SET
		   topic = excluded.topic,
		   host_id = excluded.host_id,
		   start_time = excluded.start_time,
		   join_url = excluded.join_url`,
		m.ZoomAccountID, m.MeetingID, m.Topic, m.HostID, m.StartTime, m.JoinURL,
	)
	if err != nil {
		return fmt.Errorf("upsert meeting: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetMeeting(ctx context.Context, zoomAccountID string, meetingID string) (*store.ActiveMeeting, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT zoom_account_id, meeting_id, topic, host_id, start_time, join_url
		 FROM active_meetings WHERE zoom_account_id = ? AND meeting_id = ?`, zoomAccountID, meetingID,
	)
	m := &store.ActiveMeeting{}
	err := row.Scan(&m.ZoomAccountID, &m.MeetingID, &m.Topic, &m.HostID, &m.StartTime, &m.JoinURL)
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
		`SELECT m.zoom_account_id, m.meeting_id, m.topic, m.host_id, m.start_time, m.join_url
		 FROM active_meetings m
		 JOIN tenants t ON t.zoom_account_id = m.zoom_account_id
		 WHERE t.id = ?
		 ORDER BY m.start_time, m.meeting_id`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list active meetings: %w", err)
	}
	defer closeRows(rows)

	var meetings []*store.ActiveMeeting
	for rows.Next() {
		m := &store.ActiveMeeting{}
		if err := rows.Scan(&m.ZoomAccountID, &m.MeetingID, &m.Topic, &m.HostID, &m.StartTime, &m.JoinURL); err != nil {
			return nil, fmt.Errorf("scan meeting: %w", err)
		}
		meetings = append(meetings, m)
	}
	return meetings, rows.Err()
}

// DeleteMeeting removes the meeting; its participants are removed by cascade.
func (s *SQLiteStore) DeleteMeeting(ctx context.Context, zoomAccountID string, meetingID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM active_meetings WHERE zoom_account_id = ? AND meeting_id = ?`, zoomAccountID, meetingID)
	if err != nil {
		return fmt.Errorf("delete meeting: %w", err)
	}
	return nil
}

func (s *SQLiteStore) AddParticipant(ctx context.Context, p *store.Participant) error {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO participants (zoom_account_id, meeting_id, user_name, email, join_time)
		 VALUES (?, ?, ?, ?, ?)`,
		p.ZoomAccountID, p.MeetingID, p.UserName, p.Email, p.JoinTime,
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

func (s *SQLiteStore) SetParticipantLeft(ctx context.Context, zoomAccountID string, meetingID string, userName string, leaveTime *time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE participants SET leave_time = ?
		 WHERE zoom_account_id = ? AND meeting_id = ? AND user_name = ? AND leave_time IS NULL`,
		leaveTime, zoomAccountID, meetingID, userName,
	)
	if err != nil {
		return fmt.Errorf("set participant left: %w", err)
	}
	return nil
}

const participantColumns = `id, zoom_account_id, meeting_id, user_name, email, join_time, leave_time`

func scanParticipants(rows *sql.Rows) ([]*store.Participant, error) {
	var participants []*store.Participant
	for rows.Next() {
		p := &store.Participant{}
		if err := rows.Scan(&p.ID, &p.ZoomAccountID, &p.MeetingID, &p.UserName, &p.Email, &p.JoinTime, &p.LeaveTime); err != nil {
			return nil, fmt.Errorf("scan participant: %w", err)
		}
		participants = append(participants, p)
	}
	return participants, rows.Err()
}

func (s *SQLiteStore) GetActiveParticipants(ctx context.Context, zoomAccountID string, meetingID string) ([]*store.Participant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+participantColumns+`
		 FROM participants
		 WHERE zoom_account_id = ? AND meeting_id = ? AND leave_time IS NULL
		 ORDER BY id`, zoomAccountID, meetingID,
	)
	if err != nil {
		return nil, fmt.Errorf("get active participants: %w", err)
	}
	defer closeRows(rows)
	return scanParticipants(rows)
}

func (s *SQLiteStore) GetActiveParticipantsByMeetings(ctx context.Context, zoomAccountID string, meetingIDs []string) (map[string][]*store.Participant, error) {
	if len(meetingIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(meetingIDs))
	args := make([]interface{}, 0, len(meetingIDs)+1)
	args = append(args, zoomAccountID)
	for i, id := range meetingIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(
		`SELECT `+participantColumns+`
		 FROM participants
		 WHERE zoom_account_id = ? AND meeting_id IN (%s) AND leave_time IS NULL
		 ORDER BY id`,
		strings.Join(placeholders, ","),
	)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get active participants by meetings: %w", err)
	}
	defer closeRows(rows)

	participants, err := scanParticipants(rows)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]*store.Participant)
	for _, p := range participants {
		result[p.MeetingID] = append(result[p.MeetingID], p)
	}
	return result, nil
}

func (s *SQLiteStore) DeleteParticipantsForMeeting(ctx context.Context, zoomAccountID string, meetingID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM participants WHERE zoom_account_id = ? AND meeting_id = ?`, zoomAccountID, meetingID)
	if err != nil {
		return fmt.Errorf("delete participants: %w", err)
	}
	return nil
}
