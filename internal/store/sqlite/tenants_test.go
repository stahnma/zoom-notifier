package sqlite

import (
	"context"
	"testing"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

func setupTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndGetTenant(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	tenant := &store.Tenant{
		ID:            "T_SLACK_123",
		TeamName:      "Test Team",
		APIKey:        "key-123",
		ZoomAccountID: "zoom-acct-1",
	}
	if err := s.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	got, err := s.GetTenant(ctx, "T_SLACK_123")
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got.TeamName != "Test Team" {
		t.Errorf("expected team name 'Test Team', got '%s'", got.TeamName)
	}
	if got.APIKey != "key-123" {
		t.Errorf("expected api key 'key-123', got '%s'", got.APIKey)
	}
}

func TestGetTenantByZoomAccount(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.CreateTenant(ctx, &store.Tenant{ID: "T1", APIKey: "k1", ZoomAccountID: "zoom-1"})
	s.CreateTenant(ctx, &store.Tenant{ID: "T2", APIKey: "k2", ZoomAccountID: "zoom-1"})
	s.CreateTenant(ctx, &store.Tenant{ID: "T3", APIKey: "k3", ZoomAccountID: "zoom-2"})

	tenants, err := s.GetTenantByZoomAccount(ctx, "zoom-1")
	if err != nil {
		t.Fatalf("get by zoom account: %v", err)
	}
	if len(tenants) != 2 {
		t.Errorf("expected 2 tenants, got %d", len(tenants))
	}
}

func TestDeleteTenant(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.CreateTenant(ctx, &store.Tenant{ID: "T1", APIKey: "k1"})
	if err := s.DeleteTenant(ctx, "T1"); err != nil {
		t.Fatalf("delete tenant: %v", err)
	}
	got, err := s.GetTenant(ctx, "T1")
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestListTenants(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.CreateTenant(ctx, &store.Tenant{ID: "T1", APIKey: "k1"})
	s.CreateTenant(ctx, &store.Tenant{ID: "T2", APIKey: "k2"})

	tenants, err := s.ListTenants(ctx)
	if err != nil {
		t.Fatalf("list tenants: %v", err)
	}
	if len(tenants) != 2 {
		t.Errorf("expected 2 tenants, got %d", len(tenants))
	}
}

func TestTenantDefaultFields(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create tenant with default suffix and include_link
	tenant := &store.Tenant{
		ID:                 "T_DEFAULTS",
		APIKey:             "key-defaults",
		DefaultMsgSuffix:   "the meeting.",
		DefaultIncludeLink: true,
	}
	if err := s.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	got, err := s.GetTenant(ctx, "T_DEFAULTS")
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got.DefaultMsgSuffix != "the meeting." {
		t.Errorf("expected default_msg_suffix 'the meeting.', got '%s'", got.DefaultMsgSuffix)
	}
	if !got.DefaultIncludeLink {
		t.Error("expected default_include_link=true")
	}
}

func TestUpdateTenantDefaults(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.CreateTenant(ctx, &store.Tenant{ID: "T1", APIKey: "k1", DefaultMsgSuffix: "", DefaultIncludeLink: false})

	if err := s.UpdateTenantDefaults(ctx, "T1", "the standup.", true); err != nil {
		t.Fatalf("update tenant defaults: %v", err)
	}

	got, _ := s.GetTenant(ctx, "T1")
	if got.DefaultMsgSuffix != "the standup." {
		t.Errorf("expected 'the standup.', got '%s'", got.DefaultMsgSuffix)
	}
	if !got.DefaultIncludeLink {
		t.Error("expected default_include_link=true")
	}

	// Update again to verify overwrite
	if err := s.UpdateTenantDefaults(ctx, "T1", "a zoom call.", false); err != nil {
		t.Fatalf("update tenant defaults again: %v", err)
	}
	got, _ = s.GetTenant(ctx, "T1")
	if got.DefaultMsgSuffix != "a zoom call." {
		t.Errorf("expected 'a zoom call.', got '%s'", got.DefaultMsgSuffix)
	}
	if got.DefaultIncludeLink {
		t.Error("expected default_include_link=false after second update")
	}
}

func TestUpdateTenantAPIKey(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.CreateTenant(ctx, &store.Tenant{ID: "T1", APIKey: "old-key"})
	if err := s.UpdateTenantAPIKey(ctx, "T1", "new-key"); err != nil {
		t.Fatalf("update api key: %v", err)
	}
	got, _ := s.GetTenant(ctx, "T1")
	if got.APIKey != "new-key" {
		t.Errorf("expected new-key, got %s", got.APIKey)
	}
}
