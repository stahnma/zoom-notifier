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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Log("close:", err)
		}
	}()

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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Log("close:", err)
		}
	}()

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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Log("close:", err)
		}
	}()

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

func TestSaveZoomSecret(t *testing.T) {
	h := NewHandler(filepath.Join(t.TempDir(), "config.toml"))
	srv := httptest.NewServer(h.Router())
	defer srv.Close()

	// POST with valid secret
	resp, err := http.Post(
		srv.URL+"/setup/api/save-zoom-secret",
		"application/json",
		strings.NewReader(`{"secret":"test-secret-123"}`),
	)
	if err != nil {
		t.Fatalf("POST /setup/api/save-zoom-secret failed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Log("close:", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", result["status"])
	}

	// Verify the secret was stored in handler data
	if h.data.ZoomSecret != "test-secret-123" {
		t.Errorf("expected zoom secret 'test-secret-123', got '%s'", h.data.ZoomSecret)
	}
}

func TestSaveZoomSecret_MissingSecret(t *testing.T) {
	h := NewHandler(filepath.Join(t.TempDir(), "config.toml"))
	srv := httptest.NewServer(h.Router())
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/setup/api/save-zoom-secret",
		"application/json",
		strings.NewReader(`{"secret":""}`),
	)
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Log("close:", err)
		}
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty secret, got %d", resp.StatusCode)
	}
}

func TestWebhookZoomCRC_NoSecret(t *testing.T) {
	h := NewHandler(filepath.Join(t.TempDir(), "config.toml"))
	// Clear the zoom secret to simulate unconfigured state
	h.data.ZoomSecret = ""
	srv := httptest.NewServer(h.Router())
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/webhook/zoom",
		"application/json",
		strings.NewReader(`{"event":"endpoint.url_validation","payload":{"plainToken":"abc123"}}`),
	)
	if err != nil {
		t.Fatalf("POST /webhook/zoom failed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Log("close:", err)
		}
	}()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 when no secret configured, got %d", resp.StatusCode)
	}
}

func TestWebhookZoomCRC_WithSecret(t *testing.T) {
	h := NewHandler(filepath.Join(t.TempDir(), "config.toml"))
	h.data.ZoomSecret = "my-test-secret"
	srv := httptest.NewServer(h.Router())
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/webhook/zoom",
		"application/json",
		strings.NewReader(`{"event":"endpoint.url_validation","payload":{"plainToken":"abc123"}}`),
	)
	if err != nil {
		t.Fatalf("POST /webhook/zoom failed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Log("close:", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode CRC response: %v", err)
	}
	if result["plainToken"] != "abc123" {
		t.Errorf("expected plainToken 'abc123', got '%s'", result["plainToken"])
	}
	if result["encryptedToken"] == "" {
		t.Error("expected non-empty encryptedToken")
	}
}

func TestWebhookZoomCRC_NonCRCEvent(t *testing.T) {
	h := NewHandler(filepath.Join(t.TempDir(), "config.toml"))
	h.data.ZoomSecret = "my-test-secret"
	srv := httptest.NewServer(h.Router())
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/webhook/zoom",
		"application/json",
		strings.NewReader(`{"event":"meeting.started","payload":{"account_id":"abc"}}`),
	)
	if err != nil {
		t.Fatalf("POST /webhook/zoom failed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Log("close:", err)
		}
	}()

	// Non-CRC events during setup should return 200 and be ignored
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for non-CRC event, got %d", resp.StatusCode)
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
	if err := resp.Body.Close(); err != nil {
		t.Log("close:", err)
	}
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
	if err := resp.Body.Close(); err != nil {
		t.Log("close:", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /setup/server-url: expected 200, got %d", resp.StatusCode)
	}

	// Step 3: POST /setup/zoom
	resp, err = client.PostForm(srv.URL+"/setup/zoom", url.Values{
		"zoom_secret":     {"zoom-secret-token"},
		"zoom_account_id": {"zoom-account-123"},
	})
	if err != nil {
		t.Fatalf("POST /setup/zoom failed: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Log("close:", err)
	}
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
	if err := resp.Body.Close(); err != nil {
		t.Log("close:", err)
	}
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
	if err := resp.Body.Close(); err != nil {
		t.Log("close:", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /setup/advanced: expected 200, got %d", resp.StatusCode)
	}

	// Step 6: POST /setup/save
	resp, err = client.PostForm(srv.URL+"/setup/save", url.Values{})
	if err != nil {
		t.Fatalf("POST /setup/save failed: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Log("close:", err)
	}
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
