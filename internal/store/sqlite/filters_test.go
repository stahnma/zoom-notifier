package sqlite

import (
	"context"
	"testing"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

func TestCreateAndListFilters(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	f := &store.MeetingFilter{TenantID: "T1", Pattern: "Daily Standup"}
	if err := s.CreateFilter(ctx, f); err != nil {
		t.Fatalf("create filter: %v", err)
	}
	if f.ID == 0 {
		t.Error("expected ID to be set")
	}

	s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T1", Pattern: "All Hands"})

	filters, err := s.ListFilters(ctx, "T1")
	if err != nil {
		t.Fatalf("list filters: %v", err)
	}
	if len(filters) != 2 {
		t.Errorf("expected 2 filters, got %d", len(filters))
	}
}

func TestDeleteFilter(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	f := &store.MeetingFilter{TenantID: "T1", Pattern: "Standup"}
	s.CreateFilter(ctx, f)

	if err := s.DeleteFilter(ctx, f.ID); err != nil {
		t.Fatalf("delete filter: %v", err)
	}
	filters, _ := s.ListFilters(ctx, "T1")
	if len(filters) != 0 {
		t.Errorf("expected 0 filters after delete, got %d", len(filters))
	}
}

func TestMatchesFilter_NoFilters_AllowAll(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	// No filters — should allow all
	matches, err := s.MatchesFilter(ctx, "T1", "Any Meeting")
	if err != nil {
		t.Fatalf("matches filter: %v", err)
	}
	if !matches {
		t.Error("expected true when no filters exist (allow all)")
	}
}

func TestMatchesFilter_WithFilters_MatchingTopic(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T1", Pattern: "Daily Standup"})
	s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T1", Pattern: "All Hands"})

	matches, err := s.MatchesFilter(ctx, "T1", "Daily Standup")
	if err != nil {
		t.Fatalf("matches filter: %v", err)
	}
	if !matches {
		t.Error("expected true for matching topic")
	}
}

func TestMatchesFilter_WithFilters_NonMatchingTopic(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T1", Pattern: "Daily Standup"})

	matches, err := s.MatchesFilter(ctx, "T1", "Random Meeting")
	if err != nil {
		t.Fatalf("matches filter: %v", err)
	}
	if matches {
		t.Error("expected false for non-matching topic")
	}
}
