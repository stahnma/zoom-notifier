package sqlite

import (
	"context"
	"testing"
)

func TestAddAndListAdmins(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	if err := s.AddAdmin(ctx, "T1", "U_ALICE"); err != nil {
		t.Fatalf("add admin: %v", err)
	}
	_ = s.AddAdmin(ctx, "T1", "U_BOB")

	admins, err := s.ListAdmins(ctx, "T1")
	if err != nil {
		t.Fatalf("list admins: %v", err)
	}
	if len(admins) != 2 {
		t.Errorf("expected 2 admins, got %d", len(admins))
	}
}

func TestIsAdmin(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	_ = s.AddAdmin(ctx, "T1", "U_ALICE")

	isAdmin, err := s.IsAdmin(ctx, "T1", "U_ALICE")
	if err != nil {
		t.Fatalf("is admin: %v", err)
	}
	if !isAdmin {
		t.Error("expected Alice to be admin")
	}

	isAdmin, _ = s.IsAdmin(ctx, "T1", "U_NOBODY")
	if isAdmin {
		t.Error("expected non-admin to return false")
	}
}

func TestRemoveAdmin(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	_ = s.AddAdmin(ctx, "T1", "U_ALICE")
	if err := s.RemoveAdmin(ctx, "T1", "U_ALICE"); err != nil {
		t.Fatalf("remove admin: %v", err)
	}

	isAdmin, _ := s.IsAdmin(ctx, "T1", "U_ALICE")
	if isAdmin {
		t.Error("expected Alice to no longer be admin")
	}
}
