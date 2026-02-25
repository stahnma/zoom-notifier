package slack

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store/sqlite"
)

func setupOAuthTestStore(t *testing.T) *sqlite.SQLiteStore {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	s, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOAuthInstallRedirect(t *testing.T) {
	s := setupOAuthTestStore(t)
	handler := NewOAuthHandler(OAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURI:  "https://example.com/slack/callback",
		Store:        s,
	})

	req := httptest.NewRequest(http.MethodGet, "/slack/install", nil)
	w := httptest.NewRecorder()
	handler.HandleInstall(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}

	location := w.Header().Get("Location")
	if !strings.HasPrefix(location, defaultSlackOAuthURL) {
		t.Fatalf("expected redirect to Slack OAuth URL, got %s", location)
	}
	if !strings.Contains(location, "client_id=test-client-id") {
		t.Errorf("expected client_id in redirect URL, got %s", location)
	}
	if !strings.Contains(location, "scope=commands") {
		t.Errorf("expected scopes in redirect URL, got %s", location)
	}
	if !strings.Contains(location, "redirect_uri=") {
		t.Errorf("expected redirect_uri in redirect URL, got %s", location)
	}
}

func TestOAuthCallbackSuccess(t *testing.T) {
	s := setupOAuthTestStore(t)

	// Mock Slack's OAuth V2 access endpoint
	slackMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/oauth.v2.access" {
			resp := oauthV2Response{
				OK:          true,
				AccessToken: "xoxb-test-bot-token",
				TokenType:   "bot",
			}
			resp.Team.ID = "T12345"
			resp.Team.Name = "Test Workspace"
			resp.AuthedUser.ID = "U12345"

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer slackMock.Close()

	handler := NewOAuthHandler(OAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURI:  "https://example.com/slack/callback",
		Store:        s,
		APIURL:       slackMock.URL + "/api/",
	})

	req := httptest.NewRequest(http.MethodGet, "/slack/callback?code=test-auth-code", nil)
	w := httptest.NewRecorder()
	handler.HandleCallback(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify tenant was created
	tenant, err := s.GetTenant(req.Context(), "T12345")
	if err != nil {
		t.Fatalf("failed to get tenant: %v", err)
	}
	if tenant == nil {
		t.Fatal("expected tenant to be created")
	}
	if tenant.TeamName != "Test Workspace" {
		t.Errorf("expected team name 'Test Workspace', got '%s'", tenant.TeamName)
	}
	if tenant.BotToken == nil || *tenant.BotToken != "xoxb-test-bot-token" {
		t.Error("expected bot token to be set")
	}
	if tenant.APIKey == "" {
		t.Error("expected API key to be generated")
	}
}

func TestOAuthCallbackMissingCode(t *testing.T) {
	s := setupOAuthTestStore(t)
	handler := NewOAuthHandler(OAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURI:  "https://example.com/slack/callback",
		Store:        s,
	})

	req := httptest.NewRequest(http.MethodGet, "/slack/callback", nil)
	w := httptest.NewRecorder()
	handler.HandleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestOAuthCallbackWithError(t *testing.T) {
	s := setupOAuthTestStore(t)
	handler := NewOAuthHandler(OAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURI:  "https://example.com/slack/callback",
		Store:        s,
	})

	req := httptest.NewRequest(http.MethodGet, "/slack/callback?error=access_denied", nil)
	w := httptest.NewRecorder()
	handler.HandleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOAuthCallbackInvalidCode(t *testing.T) {
	s := setupOAuthTestStore(t)

	// Mock Slack returning an error for invalid code
	slackMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/oauth.v2.access" {
			resp := oauthV2Response{
				OK:    false,
				Error: "invalid_code",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer slackMock.Close()

	handler := NewOAuthHandler(OAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURI:  "https://example.com/slack/callback",
		Store:        s,
		APIURL:       slackMock.URL + "/api/",
	})

	req := httptest.NewRequest(http.MethodGet, "/slack/callback?code=invalid-code", nil)
	w := httptest.NewRecorder()
	handler.HandleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
