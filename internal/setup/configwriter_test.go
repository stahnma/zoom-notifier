package setup

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConfig_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	data := &SetupData{
		ServerURL:          "https://example.com",
		AdminAPIKey:        "admin-key-123",
		ZoomSecret:         "zoom-secret-456",
		SlackClientID:      "slack-client-id",
		SlackClientSecret:  "slack-client-secret",
		SlackSigningSecret: "slack-signing-secret",
		ServerHost:         "0.0.0.0",
		ServerPort:         9090,
		DatabasePath:       "/var/lib/zoom-notifier/data.db",
		LogLevel:           "debug",
	}

	if err := WriteConfig(path, data); err != nil {
		t.Fatalf("WriteConfig returned error: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected config file to exist, but it does not")
	}
}

func TestWriteConfig_ContainsExpectedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	data := &SetupData{
		ServerURL:          "https://example.com",
		AdminAPIKey:        "admin-key-123",
		ZoomSecret:         "zoom-secret-456",
		SlackClientID:      "slack-client-id",
		SlackClientSecret:  "slack-client-secret",
		SlackSigningSecret: "slack-signing-secret",
		ServerHost:         "0.0.0.0",
		ServerPort:         9090,
		DatabasePath:       "/var/lib/zoom-notifier/data.db",
		LogLevel:           "debug",
	}

	if err := WriteConfig(path, data); err != nil {
		t.Fatalf("WriteConfig returned error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	body := string(content)

	expectations := []string{
		`port = 9090`,
		`host = "0.0.0.0"`,
		`path = "/var/lib/zoom-notifier/data.db"`,
		`webhook_secret = "zoom-secret-456"`,
		`client_id = "slack-client-id"`,
		`client_secret = "slack-client-secret"`,
		`signing_secret = "slack-signing-secret"`,
		`api_key = "admin-key-123"`,
		`level = "debug"`,
		`[server]`,
		`[database]`,
		`[zoom]`,
		`[slack]`,
		`[admin]`,
		`[log]`,
	}

	for _, exp := range expectations {
		if !strings.Contains(body, exp) {
			t.Errorf("config file missing expected content %q", exp)
		}
	}
}

func TestWriteConfig_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	data := &SetupData{
		ServerHost: "localhost",
		ServerPort: 8888,
		LogLevel:   "info",
	}

	if err := WriteConfig(path, data); err != nil {
		t.Fatalf("WriteConfig returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat config file: %v", err)
	}

	perm := info.Mode().Perm()
	expected := fs.FileMode(0600)
	if perm != expected {
		t.Errorf("expected file permissions %o, got %o", expected, perm)
	}
}
