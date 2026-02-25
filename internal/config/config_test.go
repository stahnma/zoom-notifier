package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 8888 {
		t.Errorf("expected default port 8888, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "localhost" {
		t.Errorf("expected default host localhost, got %s", cfg.Server.Host)
	}
	if cfg.Database.Path != "./zoom-notifier.db" {
		t.Errorf("expected default db path, got %s", cfg.Database.Path)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected default log level info, got %s", cfg.Log.Level)
	}
}

func TestLoadFromTOMLFile(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "config.toml")
	err := os.WriteFile(tomlPath, []byte(`
[server]
port = 9999
host = "0.0.0.0"

[log]
level = "debug"
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tomlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Log.Level)
	}
}

func TestLoadFromEnvVars(t *testing.T) {
	t.Setenv("ZOOM_SECRET", "testsecret")
	t.Setenv("ZOOMNOTIFIER_ADMIN_KEY", "testadminkey")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Zoom.WebhookSecret != "testsecret" {
		t.Errorf("expected zoom secret testsecret, got %s", cfg.Zoom.WebhookSecret)
	}
	if cfg.Admin.APIKey != "testadminkey" {
		t.Errorf("expected admin key testadminkey, got %s", cfg.Admin.APIKey)
	}
}
