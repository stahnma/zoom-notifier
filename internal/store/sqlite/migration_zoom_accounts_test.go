package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migsqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stahnma/zoom-notifier/internal/secrets"
)

// migrateTo moves the schema to the given version.
func migrateTo(t *testing.T, s *SQLiteStore, version uint) {
	t.Helper()
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	drv, err := migsqlite.WithInstance(s.db, &migsqlite.Config{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Migrate(version); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate to %d: %v", version, err)
	}
}

// TestMigration5_ZoomAccounts loads pre-migration data shaped like a real
// deployment — two tenants sharing one Zoom account (with the old per-tenant
// duplicate participant rows), a tenant whose account ID was only ever set
// via API credentials, and encrypted secrets — and checks the upgrade.
func TestMigration5_ZoomAccounts(t *testing.T) {
	key, err := secrets.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(filepath.Join(t.TempDir(), "m.db"), WithEncryptionKey(key))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Log("close:", err)
		}
	})
	migrateTo(t, s, 4)

	sealed, err := s.cipher.Seal("shared-secret")
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.db.Exec(q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`INSERT INTO tenants (id, team_name, api_key, zoom_account_id, installed_at) VALUES ('SLACK-A', '', 'ka', 'SHARED', '2026-01-01')`)
	exec(`INSERT INTO tenants (id, team_name, api_key, zoom_account_id, installed_at) VALUES ('SLACK-B', '', 'kb', 'SHARED', '2026-02-01')`)
	exec(`INSERT INTO tenants (id, team_name, api_key, zoom_account_id) VALUES ('APIONLY', '', 'kc', '')`)
	exec(`INSERT INTO tenants (id, team_name, api_key, zoom_account_id) VALUES ('NOZOOM', '', 'kd', NULL)`)
	exec(`INSERT INTO zoom_credentials (tenant_id, client_id, client_secret, account_id) VALUES ('SLACK-A', 'cid-a', 'stale', 'SHARED')`)
	exec(`INSERT INTO zoom_credentials (tenant_id, client_id, client_secret, account_id) VALUES ('SLACK-B', 'cid-b', ?, 'SHARED')`, sealed)
	exec(`INSERT INTO zoom_credentials (tenant_id, client_id, client_secret, account_id) VALUES ('APIONLY', 'cid-c', 'secret-c', 'API-ACCT')`)
	// Old fan-out: last tenant processed owns the meeting; one participant row per tenant.
	exec(`INSERT INTO active_meetings (meeting_id, tenant_id, topic, host_id, start_time, join_url) VALUES ('m-1', 'SLACK-B', 'Standup', 'h', '2026-03-01 09:59:00', '')`)
	exec(`INSERT INTO participants (meeting_id, user_name, email, join_time) VALUES ('m-1', 'Alice', '', '2026-03-01 10:00:00')`)
	exec(`INSERT INTO participants (meeting_id, user_name, email, join_time) VALUES ('m-1', 'Alice', '', '2026-03-01 10:00:00')`)
	exec(`INSERT INTO participants (meeting_id, user_name, email, join_time) VALUES ('m-1', 'Bob', '', '2026-03-01 10:05:00')`)

	migrateTo(t, s, 5)
	ctx := context.Background()

	// Newest tenant's credentials win for a shared account, ciphertext intact.
	shared, err := s.GetZoomAccount(ctx, "SHARED")
	if err != nil {
		t.Fatal(err)
	}
	if shared == nil || shared.ClientID != "cid-b" || shared.ClientSecret != "shared-secret" || shared.WebhookSecret != "" {
		t.Errorf("SHARED account = %+v", shared)
	}

	// API-only tenant gets linked to its credentials' account.
	apiOnly, err := s.GetTenant(ctx, "APIONLY")
	if err != nil {
		t.Fatal(err)
	}
	if apiOnly.ZoomAccountID != "API-ACCT" {
		t.Errorf("APIONLY linked to %q, want API-ACCT", apiOnly.ZoomAccountID)
	}
	if a, err := s.GetZoomAccount(ctx, "API-ACCT"); err != nil || a == nil || a.ClientSecret != "secret-c" {
		t.Errorf("API-ACCT account = %+v, %v", a, err)
	}

	accounts, err := s.ListZoomAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 {
		t.Errorf("expected 2 accounts (SHARED, API-ACCT), got %+v", accounts)
	}

	// Both tenants now see the meeting, with duplicates collapsed.
	for _, tenant := range []string{"SLACK-A", "SLACK-B"} {
		meetings, err := s.ListActiveMeetings(ctx, tenant)
		if err != nil {
			t.Fatal(err)
		}
		if len(meetings) != 1 || meetings[0].Topic != "Standup" {
			t.Fatalf("%s meetings = %+v", tenant, meetings)
		}
	}
	active, err := s.GetActiveParticipants(ctx, "SHARED", "m-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 || active[0].UserName != "Alice" || active[1].UserName != "Bob" {
		t.Errorf("expected deduplicated [Alice Bob], got %+v", active)
	}

	// Down migration restores per-tenant credentials and meetings.
	migrateTo(t, s, 4)
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM zoom_credentials WHERE account_id = 'SHARED'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("down: expected SHARED credentials copied to both tenants, got %d rows", n)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM participants WHERE meeting_id = 'm-1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("down: expected 2 participants, got %d", n)
	}

	// And back up again cleanly.
	migrateTo(t, s, 5)
}
