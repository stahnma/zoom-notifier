package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stahnma/zoom-notifier/internal/store"
)

// createLinkedTenant creates a tenant linked to zoomAccountID (which creates
// the account row).
func createLinkedTenant(t *testing.T, s *SQLiteStore, id, zoomAccountID string) {
	t.Helper()
	if err := s.CreateTenant(context.Background(), &store.Tenant{
		ID: id, APIKey: "key-" + id, ZoomAccountID: zoomAccountID,
	}); err != nil {
		t.Fatalf("create linked tenant: %v", err)
	}
}

func TestUpsertAndGetMeeting(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createLinkedTenant(t, s, "T1", "A1")

	meeting := &store.ActiveMeeting{
		ZoomAccountID: "A1",
		MeetingID:     "m-100",
		Topic:         "Daily Standup",
		HostID:        "host-1",
		StartTime:     time.Now().Truncate(time.Second),
		JoinURL:       "https://zoom.us/j/100",
	}
	if err := s.UpsertMeeting(ctx, meeting); err != nil {
		t.Fatalf("upsert meeting: %v", err)
	}

	got, err := s.GetMeeting(ctx, "A1", "m-100")
	if err != nil {
		t.Fatalf("get meeting: %v", err)
	}
	if got == nil {
		t.Fatal("expected meeting, got nil")
	}
	if got.Topic != "Daily Standup" || got.JoinURL != "https://zoom.us/j/100" || got.ZoomAccountID != "A1" {
		t.Errorf("unexpected meeting: %+v", got)
	}

	other, err := s.GetMeeting(ctx, "A2", "m-100")
	if err != nil {
		t.Fatal(err)
	}
	if other != nil {
		t.Error("meeting must not be visible under a different zoom account")
	}
}

func TestUpsertMeeting_UpdateExisting(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createLinkedTenant(t, s, "T1", "A1")

	meeting := &store.ActiveMeeting{ZoomAccountID: "A1", MeetingID: "m-100", Topic: "Old Topic"}
	if err := s.UpsertMeeting(ctx, meeting); err != nil {
		t.Fatal(err)
	}
	meeting.Topic = "New Topic"
	meeting.JoinURL = "https://zoom.us/j/updated"
	if err := s.UpsertMeeting(ctx, meeting); err != nil {
		t.Fatalf("upsert meeting (update): %v", err)
	}

	got, err := s.GetMeeting(ctx, "A1", "m-100")
	if err != nil {
		t.Fatal(err)
	}
	if got.Topic != "New Topic" || got.JoinURL != "https://zoom.us/j/updated" {
		t.Errorf("expected updated meeting, got %+v", got)
	}
}

func TestUpsertMeeting_RequiresKnownAccount(t *testing.T) {
	s := setupTestStore(t)
	err := s.UpsertMeeting(context.Background(), &store.ActiveMeeting{ZoomAccountID: "NOPE", MeetingID: "m-1"})
	if err == nil {
		t.Fatal("expected foreign key error for unknown zoom account")
	}
}

func TestListActiveMeetings(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createLinkedTenant(t, s, "T1", "A1")
	createLinkedTenant(t, s, "T2", "A2")
	createTestTenant(t, s, "T3") // no zoom account

	for _, m := range []*store.ActiveMeeting{
		{ZoomAccountID: "A1", MeetingID: "m-1", Topic: "A"},
		{ZoomAccountID: "A1", MeetingID: "m-2", Topic: "B"},
		{ZoomAccountID: "A2", MeetingID: "m-3", Topic: "C"},
	} {
		if err := s.UpsertMeeting(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	for tenant, want := range map[string]int{"T1": 2, "T2": 1, "T3": 0} {
		meetings, err := s.ListActiveMeetings(ctx, tenant)
		if err != nil {
			t.Fatalf("list active meetings for %s: %v", tenant, err)
		}
		if len(meetings) != want {
			t.Errorf("expected %d meetings for %s, got %d", want, tenant, len(meetings))
		}
	}
}

// Tenants linked to the same Zoom account see the same meetings and
// participants, with no per-tenant duplicates.
func TestSharedAccountMeetingState(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createLinkedTenant(t, s, "SLACK-A", "SHARED")
	createLinkedTenant(t, s, "SLACK-B", "SHARED")

	if err := s.UpsertMeeting(ctx, &store.ActiveMeeting{ZoomAccountID: "SHARED", MeetingID: "m-1", Topic: "Standup"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddParticipant(ctx, &store.Participant{ZoomAccountID: "SHARED", MeetingID: "m-1", UserName: "Alice", JoinTime: time.Now()}); err != nil {
		t.Fatal(err)
	}

	for _, tenant := range []string{"SLACK-A", "SLACK-B"} {
		meetings, err := s.ListActiveMeetings(ctx, tenant)
		if err != nil {
			t.Fatal(err)
		}
		if len(meetings) != 1 || meetings[0].MeetingID != "m-1" {
			t.Fatalf("%s: expected the shared meeting, got %+v", tenant, meetings)
		}
		byMeeting, err := s.GetActiveParticipantsByMeetings(ctx, meetings[0].ZoomAccountID, []string{"m-1"})
		if err != nil {
			t.Fatal(err)
		}
		if got := byMeeting["m-1"]; len(got) != 1 || got[0].UserName != "Alice" {
			t.Errorf("%s: expected exactly Alice, got %+v", tenant, got)
		}
	}
}

// The same meeting ID under two accounts is two independent meetings.
func TestMeetingIDsAreScopedByAccount(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createLinkedTenant(t, s, "T1", "A1")
	createLinkedTenant(t, s, "T2", "A2")

	for _, acct := range []string{"A1", "A2"} {
		if err := s.UpsertMeeting(ctx, &store.ActiveMeeting{ZoomAccountID: acct, MeetingID: "m-1", Topic: "topic-" + acct}); err != nil {
			t.Fatal(err)
		}
		if err := s.AddParticipant(ctx, &store.Participant{ZoomAccountID: acct, MeetingID: "m-1", UserName: "user-" + acct, JoinTime: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}

	leave := time.Now()
	if err := s.SetParticipantLeft(ctx, "A1", "m-1", "user-A1", &leave); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteMeeting(ctx, "A1", "m-1"); err != nil {
		t.Fatal(err)
	}

	m, err := s.GetMeeting(ctx, "A2", "m-1")
	if err != nil {
		t.Fatal(err)
	}
	if m == nil || m.Topic != "topic-A2" {
		t.Fatalf("A2's meeting must survive A1's delete, got %+v", m)
	}
	active, err := s.GetActiveParticipants(ctx, "A2", "m-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].UserName != "user-A2" {
		t.Errorf("A2's participants must be untouched, got %+v", active)
	}
}

func TestMeetingLifecycle(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createLinkedTenant(t, s, "T1", "A1")

	if err := s.UpsertMeeting(ctx, &store.ActiveMeeting{ZoomAccountID: "A1", MeetingID: "m-1", Topic: "Standup"}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Truncate(time.Second)
	for _, name := range []string{"Alice", "Bob"} {
		if err := s.AddParticipant(ctx, &store.Participant{ZoomAccountID: "A1", MeetingID: "m-1", UserName: name, JoinTime: now}); err != nil {
			t.Fatal(err)
		}
	}

	active, err := s.GetActiveParticipants(ctx, "A1", "m-1")
	if err != nil {
		t.Fatalf("get active participants: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("expected 2 active participants, got %d", len(active))
	}

	leaveTime := now.Add(30 * time.Minute)
	if err := s.SetParticipantLeft(ctx, "A1", "m-1", "Alice", &leaveTime); err != nil {
		t.Fatalf("set participant left: %v", err)
	}
	active, err = s.GetActiveParticipants(ctx, "A1", "m-1")
	if err != nil {
		t.Fatalf("get active participants after leave: %v", err)
	}
	if len(active) != 1 || active[0].UserName != "Bob" {
		t.Fatalf("expected only Bob active, got %+v", active)
	}

	// Delete meeting — should cascade to participants
	if err := s.DeleteMeeting(ctx, "A1", "m-1"); err != nil {
		t.Fatalf("delete meeting: %v", err)
	}
	got, err := s.GetMeeting(ctx, "A1", "m-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil meeting after delete")
	}
	active, err = s.GetActiveParticipants(ctx, "A1", "m-1")
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
	createLinkedTenant(t, s, "T1", "A1")

	if err := s.UpsertMeeting(ctx, &store.ActiveMeeting{ZoomAccountID: "A1", MeetingID: "m-1"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Alice", "Bob"} {
		if err := s.AddParticipant(ctx, &store.Participant{ZoomAccountID: "A1", MeetingID: "m-1", UserName: name, JoinTime: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.DeleteParticipantsForMeeting(ctx, "A1", "m-1"); err != nil {
		t.Fatalf("delete participants: %v", err)
	}
	active, err := s.GetActiveParticipants(ctx, "A1", "m-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0 participants, got %d", len(active))
	}
}

func TestDeleteZoomAccountCascadesMeetings(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	if err := s.EnsureZoomAccount(ctx, "A1"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertMeeting(ctx, &store.ActiveMeeting{ZoomAccountID: "A1", MeetingID: "m-1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddParticipant(ctx, &store.Participant{ZoomAccountID: "A1", MeetingID: "m-1", UserName: "Alice", JoinTime: time.Now()}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteZoomAccount(ctx, "A1"); err != nil {
		t.Fatal(err)
	}
	m, err := s.GetMeeting(ctx, "A1", "m-1")
	if err != nil {
		t.Fatal(err)
	}
	if m != nil {
		t.Error("expected meeting removed with its zoom account")
	}
	active, err := s.GetActiveParticipants(ctx, "A1", "m-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Errorf("expected participants removed with their zoom account, got %d", len(active))
	}
}
