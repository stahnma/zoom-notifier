package sqlite

import (
	"context"
	"testing"

	"github.com/stahnma/zoom-notifier/internal/store"
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

	_ = s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T1", Pattern: "All Hands"})

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
	_ = s.CreateFilter(ctx, f)

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

	_ = s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T1", Pattern: "Daily Standup"})
	_ = s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T1", Pattern: "All Hands"})

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

	_ = s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T1", Pattern: "Daily Standup"})

	matches, err := s.MatchesFilter(ctx, "T1", "Random Meeting")
	if err != nil {
		t.Fatalf("matches filter: %v", err)
	}
	if matches {
		t.Error("expected false for non-matching topic")
	}
}

func TestGetMatchingFilter_NoFilters(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	f, err := s.GetMatchingFilter(ctx, "T1", "Any Meeting")
	if err != nil {
		t.Fatalf("get matching filter: %v", err)
	}
	if f != nil {
		t.Error("expected nil when no filters exist")
	}
}

func TestGetMatchingFilter_WithOverrides(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	suffix := "the standup."
	includeLink := true
	_ = s.CreateFilter(ctx, &store.MeetingFilter{
		TenantID:    "T1",
		Pattern:     "Daily Standup",
		MsgSuffix:   &suffix,
		IncludeLink: &includeLink,
	})

	f, err := s.GetMatchingFilter(ctx, "T1", "Daily Standup")
	if err != nil {
		t.Fatalf("get matching filter: %v", err)
	}
	if f == nil {
		t.Fatal("expected non-nil filter for matching topic")
	}
	if f.Pattern != "Daily Standup" {
		t.Errorf("expected pattern 'Daily Standup', got '%s'", f.Pattern)
	}
	if f.MsgSuffix == nil || *f.MsgSuffix != "the standup." {
		t.Errorf("expected msg_suffix 'the standup.', got %v", f.MsgSuffix)
	}
	if f.IncludeLink == nil || !*f.IncludeLink {
		t.Errorf("expected include_link true, got %v", f.IncludeLink)
	}
}

func TestGetMatchingFilter_NilOverrides(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	// Filter without overrides (nil msg_suffix and include_link)
	_ = s.CreateFilter(ctx, &store.MeetingFilter{
		TenantID: "T1",
		Pattern:  "Daily Standup",
	})

	f, err := s.GetMatchingFilter(ctx, "T1", "Daily Standup")
	if err != nil {
		t.Fatalf("get matching filter: %v", err)
	}
	if f == nil {
		t.Fatal("expected non-nil filter for matching topic")
	}
	if f.MsgSuffix != nil {
		t.Errorf("expected nil msg_suffix, got '%s'", *f.MsgSuffix)
	}
	if f.IncludeLink != nil {
		t.Errorf("expected nil include_link, got %v", *f.IncludeLink)
	}
}

func TestGetMatchingFilter_NonMatchingTopic(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	_ = s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T1", Pattern: "Daily Standup"})

	f, err := s.GetMatchingFilter(ctx, "T1", "Random Meeting")
	if err != nil {
		t.Fatalf("get matching filter: %v", err)
	}
	if f != nil {
		t.Error("expected nil for non-matching topic")
	}
}

func TestUpdateFilter(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	// Create a filter with no overrides
	f := &store.MeetingFilter{TenantID: "T1", Pattern: "Standup"}
	if err := s.CreateFilter(ctx, f); err != nil {
		t.Fatalf("create filter: %v", err)
	}

	// Update the pattern
	f.Pattern = "Daily Standup"
	suffix := "the standup."
	f.MsgSuffix = &suffix
	includeLink := true
	f.IncludeLink = &includeLink

	if err := s.UpdateFilter(ctx, f); err != nil {
		t.Fatalf("update filter: %v", err)
	}

	// Verify changes persisted
	filters, err := s.ListFilters(ctx, "T1")
	if err != nil {
		t.Fatalf("list filters: %v", err)
	}
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].Pattern != "Daily Standup" {
		t.Errorf("expected pattern 'Daily Standup', got '%s'", filters[0].Pattern)
	}
	if filters[0].MsgSuffix == nil || *filters[0].MsgSuffix != "the standup." {
		t.Errorf("expected msg_suffix 'the standup.', got %v", filters[0].MsgSuffix)
	}
	if filters[0].IncludeLink == nil || !*filters[0].IncludeLink {
		t.Errorf("expected include_link true, got %v", filters[0].IncludeLink)
	}

	// Update again: clear overrides by setting to nil
	f.MsgSuffix = nil
	f.IncludeLink = nil
	if err := s.UpdateFilter(ctx, f); err != nil {
		t.Fatalf("update filter again: %v", err)
	}

	filters, _ = s.ListFilters(ctx, "T1")
	if filters[0].MsgSuffix != nil {
		t.Errorf("expected nil msg_suffix after clearing, got '%s'", *filters[0].MsgSuffix)
	}
	if filters[0].IncludeLink != nil {
		t.Errorf("expected nil include_link after clearing, got %v", *filters[0].IncludeLink)
	}
}
