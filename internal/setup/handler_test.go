package setup

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupHandlerWelcome(t *testing.T) {
	h := NewHandler(filepath.Join(t.TempDir(), "config.toml"))
	srv := httptest.NewServer(h.Router())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/setup")
	if err != nil {
		t.Fatalf("GET /setup failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected Content-Type text/html, got %s", ct)
	}
}

func TestSetupStaticAssets(t *testing.T) {
	h := NewHandler(filepath.Join(t.TempDir(), "config.toml"))
	srv := httptest.NewServer(h.Router())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/setup/static/style.css")
	if err != nil {
		t.Fatalf("GET /setup/static/style.css failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestSetupGenerateKeyEndpoint(t *testing.T) {
	h := NewHandler(filepath.Join(t.TempDir(), "config.toml"))
	srv := httptest.NewServer(h.Router())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/setup/api/generate-key")
	if err != nil {
		t.Fatalf("GET /setup/api/generate-key failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if key, ok := result["key"]; !ok || key == "" {
		t.Error("expected non-empty 'key' field in response")
	}
}

func TestSetupFullFlow(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	h := NewHandler(configPath)
	srv := httptest.NewServer(h.Router())
	defer srv.Close()

	// The default http.Client follows redirects automatically.
	// We need a client that doesn't follow redirects for POST requests
	// since our handler renders directly (no redirects).
	client := srv.Client()

	// Step 1: GET /setup — welcome page
	resp, err := client.Get(srv.URL + "/setup")
	if err != nil {
		t.Fatalf("GET /setup failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /setup: expected 200, got %d", resp.StatusCode)
	}

	// Step 2: POST /setup/server-url
	resp, err = client.PostForm(srv.URL+"/setup/server-url", url.Values{
		"server_url": {"https://zoom.example.com"},
	})
	if err != nil {
		t.Fatalf("POST /setup/server-url failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /setup/server-url: expected 200, got %d", resp.StatusCode)
	}

	// Step 3: POST /setup/zoom
	resp, err = client.PostForm(srv.URL+"/setup/zoom", url.Values{
		"zoom_secret": {"zoom-secret-token"},
	})
	if err != nil {
		t.Fatalf("POST /setup/zoom failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /setup/zoom: expected 200, got %d", resp.StatusCode)
	}

	// Step 4: POST /setup/slack
	resp, err = client.PostForm(srv.URL+"/setup/slack", url.Values{
		"slack_client_id":      {"slack-client-id"},
		"slack_client_secret":  {"slack-client-secret"},
		"slack_signing_secret": {"slack-signing-secret"},
	})
	if err != nil {
		t.Fatalf("POST /setup/slack failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /setup/slack: expected 200, got %d", resp.StatusCode)
	}

	// Step 5: POST /setup/advanced (includes optional admin API key override)
	resp, err = client.PostForm(srv.URL+"/setup/advanced", url.Values{
		"server_host":   {"0.0.0.0"},
		"server_port":   {"9999"},
		"database_path": {"./test.db"},
		"log_level":     {"debug"},
		"admin_api_key": {"test-admin-key-12345"},
	})
	if err != nil {
		t.Fatalf("POST /setup/advanced failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /setup/advanced: expected 200, got %d", resp.StatusCode)
	}

	// Step 6: POST /setup/save
	resp, err = client.PostForm(srv.URL+"/setup/save", url.Values{})
	if err != nil {
		t.Fatalf("POST /setup/save failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /setup/save: expected 200, got %d", resp.StatusCode)
	}

	// Verify config file was written
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	content := string(data)

	checks := map[string]string{
		"admin_api_key":       "test-admin-key-12345",
		"zoom webhook_secret": "zoom-secret-token",
		"slack client_id":     "slack-client-id",
		"server port":         "9999",
		"server host":         "0.0.0.0",
		"database path":       "./test.db",
		"log level":           "debug",
	}
	for desc, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("config missing %s value %q", desc, want)
		}
	}
}
