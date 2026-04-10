package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func TestUpsertAndGetMeeting(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	meeting := &store.ActiveMeeting{
		MeetingID: "m-100",
		TenantID:  "T1",
		Topic:     "Daily Standup",
		HostID:    "host-1",
		StartTime: time.Now().Truncate(time.Second),
		JoinURL:   "https://zoom.us/j/100",
	}
	if err := s.UpsertMeeting(ctx, meeting); err != nil {
		t.Fatalf("upsert meeting: %v", err)
	}

	got, err := s.GetMeeting(ctx, "m-100")
	if err != nil {
		t.Fatalf("get meeting: %v", err)
	}
	if got == nil {
		t.Fatal("expected meeting, got nil")
	}
	if got.Topic != "Daily Standup" {
		t.Errorf("expected topic 'Daily Standup', got '%s'", got.Topic)
	}
	if got.JoinURL != "https://zoom.us/j/100" {
		t.Errorf("expected join URL, got '%s'", got.JoinURL)
	}
}

func TestUpsertMeeting_UpdateExisting(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	meeting := &store.ActiveMeeting{
		MeetingID: "m-100",
		TenantID:  "T1",
		Topic:     "Old Topic",
	}
	if err := s.UpsertMeeting(ctx, meeting); err != nil {
		t.Fatal(err)
	}

	meeting.Topic = "New Topic"
	meeting.JoinURL = "https://zoom.us/j/updated"
	if err := s.UpsertMeeting(ctx, meeting); err != nil {
		t.Fatalf("upsert meeting (update): %v", err)
	}

	got, err := s.GetMeeting(ctx, "m-100")
	if err != nil {
		t.Fatal(err)
	}
	if got.Topic != "New Topic" {
		t.Errorf("expected updated topic, got '%s'", got.Topic)
	}
	if got.JoinURL != "https://zoom.us/j/updated" {
		t.Errorf("expected updated join URL, got '%s'", got.JoinURL)
	}
}

func TestListActiveMeetings(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")
	createTestTenant(t, s, "T2")

	if err := s.UpsertMeeting(ctx, &store.ActiveMeeting{MeetingID: "m-1", TenantID: "T1", Topic: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertMeeting(ctx, &store.ActiveMeeting{MeetingID: "m-2", TenantID: "T1", Topic: "B"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertMeeting(ctx, &store.ActiveMeeting{MeetingID: "m-3", TenantID: "T2", Topic: "C"}); err != nil {
		t.Fatal(err)
	}

	meetings, err := s.ListActiveMeetings(ctx, "T1")
	if err != nil {
		t.Fatalf("list active meetings: %v", err)
	}
	if len(meetings) != 2 {
		t.Errorf("expected 2 meetings for T1, got %d", len(meetings))
	}
}

func TestMeetingLifecycle(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	// Create meeting
	if err := s.UpsertMeeting(ctx, &store.ActiveMeeting{MeetingID: "m-1", TenantID: "T1", Topic: "Standup"}); err != nil {
		t.Fatal(err)
	}

	// Add participants
	now := time.Now().Truncate(time.Second)
	if err := s.AddParticipant(ctx, &store.Participant{MeetingID: "m-1", UserName: "Alice", Email: "alice@example.com", JoinTime: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddParticipant(ctx, &store.Participant{MeetingID: "m-1", UserName: "Bob", Email: "bob@example.com", JoinTime: now}); err != nil {
		t.Fatal(err)
	}

	// Get active participants — both should be active
	active, err := s.GetActiveParticipants(ctx, "m-1")
	if err != nil {
		t.Fatalf("get active participants: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("expected 2 active participants, got %d", len(active))
	}

	// Alice leaves
	leaveTime := now.Add(30 * time.Minute)
	if err := s.SetParticipantLeft(ctx, "m-1", "Alice", &leaveTime); err != nil {
		t.Fatalf("set participant left: %v", err)
	}

	// Only Bob should be active now
	active, err = s.GetActiveParticipants(ctx, "m-1")
	if err != nil {
		t.Fatalf("get active participants after leave: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active participant, got %d", len(active))
	}
	if active[0].UserName != "Bob" {
		t.Errorf("expected Bob, got '%s'", active[0].UserName)
	}

	// Delete meeting — should cascade to participants
	if err := s.DeleteMeeting(ctx, "m-1"); err != nil {
		t.Fatalf("delete meeting: %v", err)
	}
	got, err := s.GetMeeting(ctx, "m-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil meeting after delete")
	}
	active, err = s.GetActiveParticipants(ctx, "m-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0 participants after meeting delete, got %d", len(active))
	}
}

func TestDeleteParticipantsForMeeting(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	if err := s.UpsertMeeting(ctx, &store.ActiveMeeting{MeetingID: "m-1", TenantID: "T1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddParticipant(ctx, &store.Participant{MeetingID: "m-1", UserName: "Alice", JoinTime: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddParticipant(ctx, &store.Participant{MeetingID: "m-1", UserName: "Bob", JoinTime: time.Now()}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteParticipantsForMeeting(ctx, "m-1"); err != nil {
		t.Fatalf("delete participants: %v", err)
	}
	active, err := s.GetActiveParticipants(ctx, "m-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0 participants, got %d", len(active))
	}
}
