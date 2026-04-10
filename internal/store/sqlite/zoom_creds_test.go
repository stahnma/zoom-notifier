package sqlite

import (
	"context"
	"testing"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func TestUpsertAndGetZoomCredentials(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	creds := &store.ZoomCredentials{
		TenantID:     "T1",
		ClientID:     "client-123",
		ClientSecret: "secret-456",
		AccountID:    "acct-789",
	}
	if err := s.UpsertZoomCredentials(ctx, creds); err != nil {
		t.Fatalf("upsert zoom credentials: %v", err)
	}

	got, err := s.GetZoomCredentials(ctx, "T1")
	if err != nil {
		t.Fatalf("get zoom credentials: %v", err)
	}
	if got == nil {
		t.Fatal("expected credentials, got nil")
	}
	if got.ClientID != "client-123" {
		t.Errorf("expected client_id 'client-123', got '%s'", got.ClientID)
	}
	if got.ClientSecret != "secret-456" {
		t.Errorf("expected client_secret 'secret-456', got '%s'", got.ClientSecret)
	}
	if got.AccountID != "acct-789" {
		t.Errorf("expected account_id 'acct-789', got '%s'", got.AccountID)
	}
}

func TestUpsertZoomCredentials_Update(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	s.UpsertZoomCredentials(ctx, &store.ZoomCredentials{
		TenantID: "T1", ClientID: "old-id", ClientSecret: "old-secret", AccountID: "old-acct",
	})

	s.UpsertZoomCredentials(ctx, &store.ZoomCredentials{
		TenantID: "T1", ClientID: "new-id", ClientSecret: "new-secret", AccountID: "new-acct",
	})

	got, _ := s.GetZoomCredentials(ctx, "T1")
	if got.ClientID != "new-id" {
		t.Errorf("expected updated client_id, got '%s'", got.ClientID)
	}
}

func TestGetZoomCredentials_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	got, err := s.GetZoomCredentials(ctx, "NONEXISTENT")
	if err != nil {
		t.Fatalf("get zoom credentials: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent tenant")
	}
}
