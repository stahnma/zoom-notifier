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
	if cfg.Log.Format != "text" {
		t.Errorf("expected default log format text, got %s", cfg.Log.Format)
	}
}

func TestLoadLogFormatFromTOML(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "config.toml")
	err := os.WriteFile(tomlPath, []byte(`
[log]
format = "json"
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tomlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("expected log format json, got %s", cfg.Log.Format)
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

func validConfig() *Config {
	return &Config{
		Zoom:  ZoomConfig{WebhookSecret: "secret", AccountID: "acct"},
		Slack: SlackConfig{ClientID: "cid", ClientSecret: "csec", SigningSecret: "ssec"},
		Admin: AdminConfig{APIKey: "key"},
	}
}

func TestValidate_AllPresent(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidate_MissingWebhookSecret(t *testing.T) {
	cfg := validConfig()
	cfg.Zoom.WebhookSecret = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing webhook secret")
	}
	if !contains(err.Error(), "zoom.webhook_secret") {
		t.Errorf("expected error to mention zoom.webhook_secret, got: %v", err)
	}
}

func TestValidate_MissingAccountID(t *testing.T) {
	cfg := validConfig()
	cfg.Zoom.AccountID = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing account ID")
	}
	if !contains(err.Error(), "zoom.account_id") {
		t.Errorf("expected error to mention zoom.account_id, got: %v", err)
	}
}

func TestValidate_MissingSlackClientID(t *testing.T) {
	cfg := validConfig()
	cfg.Slack.ClientID = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing Slack client ID")
	}
	if !contains(err.Error(), "slack.client_id") {
		t.Errorf("expected error to mention slack.client_id, got: %v", err)
	}
}

func TestValidate_MissingSlackClientSecret(t *testing.T) {
	cfg := validConfig()
	cfg.Slack.ClientSecret = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing Slack client secret")
	}
	if !contains(err.Error(), "slack.client_secret") {
		t.Errorf("expected error to mention slack.client_secret, got: %v", err)
	}
}

func TestValidate_MissingSigningSecret(t *testing.T) {
	cfg := validConfig()
	cfg.Slack.SigningSecret = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing signing secret")
	}
	if !contains(err.Error(), "slack.signing_secret") {
		t.Errorf("expected error to mention slack.signing_secret, got: %v", err)
	}
}

func TestValidate_MissingAdminKey(t *testing.T) {
	cfg := validConfig()
	cfg.Admin.APIKey = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing admin key")
	}
	if !contains(err.Error(), "admin.api_key") {
		t.Errorf("expected error to mention admin.api_key, got: %v", err)
	}
}

func TestValidate_MultipleFieldsMissing(t *testing.T) {
	cfg := &Config{}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty config")
	}
	// Should mention all missing fields
	for _, field := range []string{"zoom.webhook_secret", "zoom.account_id", "slack.client_id", "slack.client_secret", "slack.signing_secret", "admin.api_key"} {
		if !contains(err.Error(), field) {
			t.Errorf("expected error to mention %s, got: %v", field, err)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
