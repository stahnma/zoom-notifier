package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func TestUpsertAndGetZoomAccount(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	if err := s.UpsertZoomAccount(ctx, &store.ZoomAccount{
		AccountID: "acct-1", WebhookSecret: "wh-secret", ClientID: "client-123", ClientSecret: "secret-456",
	}); err != nil {
		t.Fatalf("upsert zoom account: %v", err)
	}

	got, err := s.GetZoomAccount(ctx, "acct-1")
	if err != nil {
		t.Fatalf("get zoom account: %v", err)
	}
	if got == nil {
		t.Fatal("expected account, got nil")
	}
	if got.WebhookSecret != "wh-secret" || got.ClientID != "client-123" || got.ClientSecret != "secret-456" {
		t.Errorf("unexpected account: %+v", got)
	}
	if !got.HasAPICredentials() {
		t.Error("expected HasAPICredentials")
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected created_at to be set")
	}
}

func TestUpsertZoomAccount_Update(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	if err := s.UpsertZoomAccount(ctx, &store.ZoomAccount{AccountID: "a", ClientID: "old", ClientSecret: "old-s"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertZoomAccount(ctx, &store.ZoomAccount{AccountID: "a", WebhookSecret: "wh", ClientID: "new", ClientSecret: "new-s"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetZoomAccount(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientID != "new" || got.ClientSecret != "new-s" || got.WebhookSecret != "wh" {
		t.Errorf("expected updated account, got %+v", got)
	}
}

func TestGetZoomAccount_NotFound(t *testing.T) {
	s := setupTestStore(t)
	got, err := s.GetZoomAccount(context.Background(), "missing")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestEnsureZoomAccount_DoesNotOverwrite(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	if err := s.UpsertZoomAccount(ctx, &store.ZoomAccount{AccountID: "a", WebhookSecret: "keep"}); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureZoomAccount(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureZoomAccount(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureZoomAccount(ctx, ""); err != nil {
		t.Fatal(err)
	}

	accounts, err := s.ListZoomAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 || accounts[0].AccountID != "a" || accounts[1].AccountID != "b" {
		t.Fatalf("expected accounts [a b], got %+v", accounts)
	}
	if accounts[0].WebhookSecret != "keep" {
		t.Error("EnsureZoomAccount must not overwrite an existing account")
	}
	if accounts[1].HasAPICredentials() {
		t.Error("ensured account should be empty")
	}
}

func TestTenantLinkCreatesZoomAccount(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	createLinkedTenant(t, s, "T1", "A1")
	if a, err := s.GetZoomAccount(ctx, "A1"); err != nil || a == nil {
		t.Fatalf("CreateTenant should create the linked account row: %+v, %v", a, err)
	}

	tenant, err := s.GetTenant(ctx, "T1")
	if err != nil {
		t.Fatal(err)
	}
	tenant.ZoomAccountID = "A2"
	if err := s.UpdateTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	if a, err := s.GetZoomAccount(ctx, "A2"); err != nil || a == nil {
		t.Fatalf("UpdateTenant should create the linked account row: %+v, %v", a, err)
	}
}

func TestSelfServiceZoomAccount(t *testing.T) {
	ctx := context.Background()

	t.Run("links and sets credentials", func(t *testing.T) {
		s := setupTestStore(t)
		createTestTenant(t, s, "T1")
		tenant, err := s.GetTenant(ctx, "T1")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SelfServiceZoomAccount(ctx, s, tenant, "A1", "cid", "csecret"); err != nil {
			t.Fatal(err)
		}
		reloaded, err := s.GetTenant(ctx, "T1")
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.ZoomAccountID != "A1" {
			t.Errorf("expected tenant linked to A1, got %q", reloaded.ZoomAccountID)
		}
		a, err := s.GetZoomAccount(ctx, "A1")
		if err != nil {
			t.Fatal(err)
		}
		if a.ClientID != "cid" || a.ClientSecret != "csecret" {
			t.Errorf("unexpected account: %+v", a)
		}

		// Blank secret keeps the existing one.
		if err := store.SelfServiceZoomAccount(ctx, s, reloaded, "A1", "cid2", ""); err != nil {
			t.Fatal(err)
		}
		a, err = s.GetZoomAccount(ctx, "A1")
		if err != nil {
			t.Fatal(err)
		}
		if a.ClientID != "cid2" || a.ClientSecret != "csecret" {
			t.Errorf("expected client ID updated and secret kept, got %+v", a)
		}
	})

	t.Run("refuses account linked to another tenant", func(t *testing.T) {
		s := setupTestStore(t)
		createLinkedTenant(t, s, "OWNER", "A1")
		if err := s.UpsertZoomAccount(ctx, &store.ZoomAccount{AccountID: "A1", ClientID: "owner-cid", ClientSecret: "owner-secret"}); err != nil {
			t.Fatal(err)
		}
		createTestTenant(t, s, "INTRUDER")
		intruder, err := s.GetTenant(ctx, "INTRUDER")
		if err != nil {
			t.Fatal(err)
		}

		err = store.SelfServiceZoomAccount(ctx, s, intruder, "A1", "evil-cid", "evil-secret")
		if !errors.Is(err, store.ErrZoomAccountManaged) {
			t.Fatalf("expected ErrZoomAccountManaged, got %v", err)
		}
		reloaded, err := s.GetTenant(ctx, "INTRUDER")
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.ZoomAccountID != "" {
			t.Error("refused tenant must not be linked")
		}
		a, err := s.GetZoomAccount(ctx, "A1")
		if err != nil {
			t.Fatal(err)
		}
		if a.ClientID != "owner-cid" || a.ClientSecret != "owner-secret" {
			t.Errorf("owner's credentials must be untouched, got %+v", a)
		}
	})

	t.Run("refuses account with operator webhook secret", func(t *testing.T) {
		s := setupTestStore(t)
		if err := s.UpsertZoomAccount(ctx, &store.ZoomAccount{AccountID: "A1", WebhookSecret: "operator"}); err != nil {
			t.Fatal(err)
		}
		createTestTenant(t, s, "T1")
		tenant, err := s.GetTenant(ctx, "T1")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SelfServiceZoomAccount(ctx, s, tenant, "A1", "", ""); !errors.Is(err, store.ErrZoomAccountManaged) {
			t.Fatalf("expected ErrZoomAccountManaged, got %v", err)
		}
	})

	t.Run("requires account ID", func(t *testing.T) {
		s := setupTestStore(t)
		createTestTenant(t, s, "T1")
		tenant, err := s.GetTenant(ctx, "T1")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SelfServiceZoomAccount(ctx, s, tenant, "", "cid", "secret"); err == nil {
			t.Fatal("expected error for empty account ID")
		}
	})
}
