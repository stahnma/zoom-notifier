package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/stahnma/zoom-notifier/internal/secrets"
	"github.com/stahnma/zoom-notifier/internal/store"
)

// setupEncryptedStore opens an in-memory store with encryption enabled and
// returns it along with the key so a second store can be opened on the same
// data if needed.
func setupEncryptedStore(t *testing.T) (*SQLiteStore, string) {
	t.Helper()
	key, err := secrets.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	s, err := New(":memory:", WithEncryptionKey(key))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Log("close:", err)
		}
	})
	return s, key
}

// rawColumn reads a column straight from the database, bypassing decryption.
func rawColumn(t *testing.T, s *SQLiteStore, query string, args ...any) string {
	t.Helper()
	var v string
	if err := s.db.QueryRow(query, args...).Scan(&v); err != nil {
		t.Fatalf("raw query %q: %v", query, err)
	}
	return v
}

func assertEncrypted(t *testing.T, label, raw, plaintext string) {
	t.Helper()
	if !secrets.IsEncrypted(raw) {
		t.Errorf("%s: expected encrypted value in db, got %q", label, raw)
	}
	if strings.Contains(raw, plaintext) {
		t.Errorf("%s: plaintext %q leaked into db value %q", label, plaintext, raw)
	}
}

func TestWithEncryptionKeyRejectsBadKey(t *testing.T) {
	if _, err := New(":memory:", WithEncryptionKey("nope")); err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestEncryptionDisabledByDefault(t *testing.T) {
	s := setupTestStore(t)
	if s.EncryptionEnabled() {
		t.Fatal("expected encryption disabled without a key")
	}
	n, err := s.EncryptLegacySecrets(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("EncryptLegacySecrets without key = %d, %v; want 0, nil", n, err)
	}
}

func TestTenantSecretsEncryptedAtRest(t *testing.T) {
	s, _ := setupEncryptedStore(t)
	ctx := context.Background()
	if !s.EncryptionEnabled() {
		t.Fatal("expected encryption enabled")
	}

	token := "xoxb-bot-token-123"
	if err := s.CreateTenant(ctx, &store.Tenant{ID: "T1", TeamName: "Team", BotToken: &token, APIKey: "api-key-abc"}); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	assertEncrypted(t, "bot_token", rawColumn(t, s, `SELECT bot_token FROM tenants WHERE id = 'T1'`), token)
	assertEncrypted(t, "api_key", rawColumn(t, s, `SELECT api_key FROM tenants WHERE id = 'T1'`), "api-key-abc")

	got, err := s.GetTenant(ctx, "T1")
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got.BotToken == nil || *got.BotToken != token {
		t.Errorf("GetTenant bot token = %v, want %q", got.BotToken, token)
	}
	if got.APIKey != "api-key-abc" {
		t.Errorf("GetTenant api key = %q", got.APIKey)
	}

	// List paths decrypt too.
	list, err := s.ListTenants(ctx)
	if err != nil || len(list) != 1 || list[0].APIKey != "api-key-abc" {
		t.Errorf("ListTenants = %+v, %v", list, err)
	}

	// Update paths re-encrypt.
	newToken := "xoxb-rotated"
	got.BotToken = &newToken
	if err := s.UpdateTenant(ctx, got); err != nil {
		t.Fatalf("update tenant: %v", err)
	}
	assertEncrypted(t, "bot_token after update", rawColumn(t, s, `SELECT bot_token FROM tenants WHERE id = 'T1'`), newToken)
	if err := s.UpdateTenantAPIKey(ctx, "T1", "api-key-new"); err != nil {
		t.Fatalf("update api key: %v", err)
	}
	assertEncrypted(t, "api_key after rotate", rawColumn(t, s, `SELECT api_key FROM tenants WHERE id = 'T1'`), "api-key-new")
	got, err = s.GetTenant(ctx, "T1")
	if err != nil || *got.BotToken != newToken || got.APIKey != "api-key-new" {
		t.Errorf("after updates: %+v, %v", got, err)
	}
}

func TestNilBotTokenStaysNull(t *testing.T) {
	s, _ := setupEncryptedStore(t)
	ctx := context.Background()
	if err := s.CreateTenant(ctx, &store.Tenant{ID: "IRC1", APIKey: "k"}); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	var isNull bool
	if err := s.db.QueryRow(`SELECT bot_token IS NULL FROM tenants WHERE id = 'IRC1'`).Scan(&isNull); err != nil {
		t.Fatal(err)
	}
	if !isNull {
		t.Error("expected NULL bot_token for IRC-only tenant")
	}
	got, err := s.GetTenant(ctx, "IRC1")
	if err != nil || got.BotToken != nil {
		t.Errorf("GetTenant = %+v, %v; want nil bot token", got, err)
	}
}

func TestZoomAndIRCSecretsEncryptedAtRest(t *testing.T) {
	s, _ := setupEncryptedStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	if err := s.UpsertZoomCredentials(ctx, &store.ZoomCredentials{TenantID: "T1", ClientID: "cid", ClientSecret: "zoom-secret", AccountID: "acct"}); err != nil {
		t.Fatalf("upsert zoom creds: %v", err)
	}
	assertEncrypted(t, "client_secret", rawColumn(t, s, `SELECT client_secret FROM zoom_credentials WHERE tenant_id = 'T1'`), "zoom-secret")
	z, err := s.GetZoomCredentials(ctx, "T1")
	if err != nil || z.ClientSecret != "zoom-secret" || z.ClientID != "cid" {
		t.Errorf("GetZoomCredentials = %+v, %v", z, err)
	}

	if err := s.UpsertIRCConfig(ctx, &store.IRCConfig{TenantID: "T1", Server: "irc.example.com:6697", Nick: "bot", Password: "irc-pass", UseTLS: true}); err != nil {
		t.Fatalf("upsert irc config: %v", err)
	}
	assertEncrypted(t, "password", rawColumn(t, s, `SELECT password FROM irc_configs WHERE tenant_id = 'T1'`), "irc-pass")
	cfgs, err := s.GetIRCConfig(ctx, "T1")
	if err != nil || len(cfgs) != 1 || cfgs[0].Password != "irc-pass" {
		t.Errorf("GetIRCConfig = %+v, %v", cfgs, err)
	}

	// Empty IRC password stays empty rather than becoming an encrypted blob.
	if err := s.UpsertIRCConfig(ctx, &store.IRCConfig{TenantID: "T1", Server: "irc2.example.com", Nick: "bot"}); err != nil {
		t.Fatalf("upsert irc config (no password): %v", err)
	}
	if raw := rawColumn(t, s, `SELECT password FROM irc_configs WHERE server = 'irc2.example.com'`); raw != "" {
		t.Errorf("expected empty password stored as empty, got %q", raw)
	}
}

func TestEncryptLegacySecrets(t *testing.T) {
	s, _ := setupEncryptedStore(t)
	ctx := context.Background()

	// Simulate rows written before encryption was enabled.
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.db.Exec(q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO tenants (id, team_name, bot_token, api_key, zoom_account_id) VALUES ('OLD', 'Old', 'xoxb-legacy', 'legacy-key', '')`)
	mustExec(`INSERT INTO tenants (id, team_name, bot_token, api_key, zoom_account_id) VALUES ('IRCONLY', 'IRC', NULL, 'irc-key', '')`)
	mustExec(`INSERT INTO zoom_credentials (tenant_id, client_id, client_secret, account_id) VALUES ('OLD', 'cid', 'legacy-zoom-secret', 'acct')`)
	mustExec(`INSERT INTO irc_configs (tenant_id, server, nick, password, use_tls) VALUES ('OLD', 'irc.a', 'n', 'legacy-irc-pass', 1)`)
	mustExec(`INSERT INTO irc_configs (tenant_id, server, nick, password, use_tls) VALUES ('OLD', 'irc.b', 'n', '', 1)`)

	// Legacy plaintext must still be readable before migration.
	tenant, err := s.GetTenant(ctx, "OLD")
	if err != nil || *tenant.BotToken != "xoxb-legacy" || tenant.APIKey != "legacy-key" {
		t.Fatalf("pre-migration read: %+v, %v", tenant, err)
	}

	n, err := s.EncryptLegacySecrets(ctx)
	if err != nil {
		t.Fatalf("EncryptLegacySecrets: %v", err)
	}
	// bot_token(OLD), api_key(OLD), api_key(IRCONLY), client_secret, password(irc.a) = 5
	if n != 5 {
		t.Errorf("updated %d rows, want 5", n)
	}

	assertEncrypted(t, "bot_token", rawColumn(t, s, `SELECT bot_token FROM tenants WHERE id = 'OLD'`), "xoxb-legacy")
	assertEncrypted(t, "api_key", rawColumn(t, s, `SELECT api_key FROM tenants WHERE id = 'OLD'`), "legacy-key")
	assertEncrypted(t, "api_key IRCONLY", rawColumn(t, s, `SELECT api_key FROM tenants WHERE id = 'IRCONLY'`), "irc-key")
	assertEncrypted(t, "client_secret", rawColumn(t, s, `SELECT client_secret FROM zoom_credentials WHERE tenant_id = 'OLD'`), "legacy-zoom-secret")
	assertEncrypted(t, "password", rawColumn(t, s, `SELECT password FROM irc_configs WHERE server = 'irc.a'`), "legacy-irc-pass")
	if raw := rawColumn(t, s, `SELECT password FROM irc_configs WHERE server = 'irc.b'`); raw != "" {
		t.Errorf("empty password should be left alone, got %q", raw)
	}

	// Everything still reads back as plaintext through the store.
	tenant, err = s.GetTenant(ctx, "OLD")
	if err != nil || *tenant.BotToken != "xoxb-legacy" || tenant.APIKey != "legacy-key" {
		t.Errorf("post-migration read: %+v, %v", tenant, err)
	}
	z, err := s.GetZoomCredentials(ctx, "OLD")
	if err != nil || z.ClientSecret != "legacy-zoom-secret" {
		t.Errorf("post-migration zoom: %+v, %v", z, err)
	}
	cfgs, err := s.GetIRCConfig(ctx, "OLD")
	if err != nil || len(cfgs) != 2 {
		t.Fatalf("post-migration irc: %+v, %v", cfgs, err)
	}

	// Idempotent: a second run touches nothing.
	n, err = s.EncryptLegacySecrets(ctx)
	if err != nil || n != 0 {
		t.Errorf("second run = %d, %v; want 0, nil", n, err)
	}
}

func TestReadEncryptedWithoutKeyFails(t *testing.T) {
	dbPath := t.TempDir() + "/enc.db"
	key, err := secrets.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := New(dbPath, WithEncryptionKey(key))
	if err != nil {
		t.Fatal(err)
	}
	if err := enc.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := enc.CreateTenant(ctx, &store.Tenant{ID: "T1", APIKey: "k"}); err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}

	plain, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := plain.Close(); err != nil {
			t.Log("close:", err)
		}
	}()
	if _, err := plain.GetTenant(ctx, "T1"); err == nil {
		t.Fatal("expected error reading encrypted tenant without a key")
	}

	wrongKey, err := secrets.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := New(dbPath, WithEncryptionKey(wrongKey))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := wrong.Close(); err != nil {
			t.Log("close:", err)
		}
	}()
	if _, err := wrong.GetTenant(ctx, "T1"); err == nil {
		t.Fatal("expected error reading encrypted tenant with wrong key")
	}
}
