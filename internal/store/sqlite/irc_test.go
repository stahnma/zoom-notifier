package sqlite

import (
	"context"
	"testing"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func TestUpsertAndGetIRCConfig(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	cfg := &store.IRCConfig{
		TenantID: "T1",
		Server:   "irc.libera.chat:6697",
		Nick:     "zoombot",
		Password: "secret",
		UseTLS:   true,
	}
	if err := s.UpsertIRCConfig(ctx, cfg); err != nil {
		t.Fatalf("upsert irc config: %v", err)
	}

	configs, err := s.GetIRCConfig(ctx, "T1")
	if err != nil {
		t.Fatalf("get irc config: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if configs[0].Server != "irc.libera.chat:6697" {
		t.Errorf("expected server 'irc.libera.chat:6697', got '%s'", configs[0].Server)
	}
	if configs[0].Nick != "zoombot" {
		t.Errorf("expected nick 'zoombot', got '%s'", configs[0].Nick)
	}
}

func TestUpsertIRCConfig_UpdateExisting(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	cfg := &store.IRCConfig{
		TenantID: "T1",
		Server:   "irc.libera.chat:6697",
		Nick:     "oldnick",
		Password: "oldpass",
		UseTLS:   true,
	}
	if err := s.UpsertIRCConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	cfg.Nick = "newnick"
	cfg.Password = "newpass"
	if err := s.UpsertIRCConfig(ctx, cfg); err != nil {
		t.Fatalf("upsert irc config (update): %v", err)
	}

	configs, err := s.GetIRCConfig(ctx, "T1")
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config after upsert, got %d", len(configs))
	}
	if configs[0].Nick != "newnick" {
		t.Errorf("expected updated nick, got '%s'", configs[0].Nick)
	}
}

func TestGetIRCConfig_MultipleServers(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestTenant(t, s, "T1")

	if err := s.UpsertIRCConfig(ctx, &store.IRCConfig{TenantID: "T1", Server: "irc.libera.chat:6697", Nick: "bot1", Password: "p1", UseTLS: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertIRCConfig(ctx, &store.IRCConfig{TenantID: "T1", Server: "irc.oftc.net:6697", Nick: "bot2", Password: "p2", UseTLS: true}); err != nil {
		t.Fatal(err)
	}

	configs, err := s.GetIRCConfig(ctx, "T1")
	if err != nil {
		t.Fatalf("get irc config: %v", err)
	}
	if len(configs) != 2 {
		t.Errorf("expected 2 configs, got %d", len(configs))
	}
}
