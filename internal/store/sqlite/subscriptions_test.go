package sqlite

import (
	"context"
	"testing"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func createTestTenant(t *testing.T, s *SQLiteStore, id string) {
	t.Helper()
	ctx := context.Background()
	err := s.CreateTenant(ctx, &store.Tenant{ID: id, APIKey: "key-" + id})
	if err != nil {
		t.Fatalf("create test tenant: %v", err)
	}
}

func TestCreateAndGetSubscription(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	meetingID := "meeting-123"
	sub := &store.Subscription{
		TenantID:  "T1",
		Type:      "slack",
		MeetingID: &meetingID,
		Target:    "#general",
		Enabled:   true,
	}
	if err := s.CreateSubscription(ctx, sub); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if sub.ID == 0 {
		t.Error("expected ID to be set after create")
	}

	got, err := s.GetSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if got.Target != "#general" {
		t.Errorf("expected target '#general', got '%s'", got.Target)
	}
	if *got.MeetingID != "meeting-123" {
		t.Errorf("expected meeting_id 'meeting-123', got '%s'", *got.MeetingID)
	}
}

func TestListSubscriptions(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")
	createTestTenant(t, s, "T2")

	s.CreateSubscription(ctx, &store.Subscription{TenantID: "T1", Type: "slack", Target: "#a", Enabled: true})
	s.CreateSubscription(ctx, &store.Subscription{TenantID: "T1", Type: "irc", Target: "#b", Enabled: true})
	s.CreateSubscription(ctx, &store.Subscription{TenantID: "T2", Type: "slack", Target: "#c", Enabled: true})

	subs, err := s.ListSubscriptions(ctx, "T1")
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(subs) != 2 {
		t.Errorf("expected 2 subscriptions for T1, got %d", len(subs))
	}
}

func TestGetSubscriptionsForMeeting_WildcardMatch(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	meetingID := "meeting-123"
	// Sub with specific meeting ID
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", MeetingID: &meetingID,
		Target: "#specific", Enabled: true,
	})
	// Sub with NULL meeting ID (wildcard — matches all)
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", MeetingID: nil,
		Target: "#wildcard", Enabled: true,
	})
	// Sub for a different meeting
	otherMeeting := "meeting-999"
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", MeetingID: &otherMeeting,
		Target: "#other", Enabled: true,
	})

	subs, err := s.GetSubscriptionsForMeeting(ctx, "T1", "meeting-123")
	if err != nil {
		t.Fatalf("get subscriptions for meeting: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 subscriptions (specific + wildcard), got %d", len(subs))
	}

	targets := map[string]bool{}
	for _, sub := range subs {
		targets[sub.Target] = true
	}
	if !targets["#specific"] {
		t.Error("expected #specific subscription")
	}
	if !targets["#wildcard"] {
		t.Error("expected #wildcard subscription")
	}
}

func TestGetSubscriptionsForMeeting_DisabledExcluded(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", MeetingID: nil,
		Target: "#disabled", Enabled: false,
	})

	subs, err := s.GetSubscriptionsForMeeting(ctx, "T1", "meeting-123")
	if err != nil {
		t.Fatalf("get subscriptions: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("expected 0 subscriptions (disabled), got %d", len(subs))
	}
}

func TestUpdateSubscription(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	sub := &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#old",
		Enabled: true,
	}
	s.CreateSubscription(ctx, sub)

	sub.Target = "#new"
	sub.Enabled = false
	if err := s.UpdateSubscription(ctx, sub); err != nil {
		t.Fatalf("update subscription: %v", err)
	}

	got, _ := s.GetSubscription(ctx, sub.ID)
	if got.Target != "#new" {
		t.Errorf("expected target '#new', got '%s'", got.Target)
	}
	if got.Enabled {
		t.Error("expected enabled=false")
	}
}

func TestDeleteSubscription(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	sub := &store.Subscription{TenantID: "T1", Type: "slack", Target: "#a", Enabled: true}
	s.CreateSubscription(ctx, sub)

	if err := s.DeleteSubscription(ctx, sub.ID); err != nil {
		t.Fatalf("delete subscription: %v", err)
	}
	got, err := s.GetSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}
