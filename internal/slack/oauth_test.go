package slack

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stahnma/zoom-notifier/internal/store/sqlite"
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
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Log("close:", err)
		}
	})
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
	if !strings.Contains(location, "state=") {
		t.Errorf("expected state param in redirect URL, got %s", location)
	}

	// Verify state cookie is set
	cookies := w.Result().Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "oauth_state" {
			stateCookie = c
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("expected oauth_state cookie to be set")
	}
	if stateCookie.Value == "" {
		t.Error("expected non-empty oauth_state cookie value")
	}
	if !stateCookie.HttpOnly {
		t.Error("expected oauth_state cookie to be HttpOnly")
	}
}

// extractOAuthState performs an install request and returns the state value from the cookie.
func extractOAuthState(t *testing.T, handler *OAuthHandler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/slack/install", nil)
	w := httptest.NewRecorder()
	handler.HandleInstall(w, req)
	for _, c := range w.Result().Cookies() {
		if c.Name == "oauth_state" {
			return c.Value
		}
	}
	t.Fatal("oauth_state cookie not found")
	return ""
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
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Log("encode:", err)
			}
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

	state := extractOAuthState(t, handler)
	req := httptest.NewRequest(http.MethodGet, "/slack/callback?code=test-auth-code&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
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

func TestOAuthCallbackMissingState(t *testing.T) {
	s := setupOAuthTestStore(t)
	handler := NewOAuthHandler(OAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURI:  "https://example.com/slack/callback",
		Store:        s,
	})

	req := httptest.NewRequest(http.MethodGet, "/slack/callback?code=test-auth-code", nil)
	w := httptest.NewRecorder()
	handler.HandleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing state, got %d", w.Code)
	}
}

func TestOAuthCallbackStateMismatch(t *testing.T) {
	s := setupOAuthTestStore(t)
	handler := NewOAuthHandler(OAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURI:  "https://example.com/slack/callback",
		Store:        s,
	})

	req := httptest.NewRequest(http.MethodGet, "/slack/callback?code=test-auth-code&state=wrong-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "correct-state"})
	w := httptest.NewRecorder()
	handler.HandleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for state mismatch, got %d", w.Code)
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

	state := extractOAuthState(t, handler)
	req := httptest.NewRequest(http.MethodGet, "/slack/callback?state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
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

	state := extractOAuthState(t, handler)
	req := httptest.NewRequest(http.MethodGet, "/slack/callback?error=access_denied&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
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
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Log("encode:", err)
			}
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

	state := extractOAuthState(t, handler)
	req := httptest.NewRequest(http.MethodGet, "/slack/callback?code=invalid-code&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	w := httptest.NewRecorder()
	handler.HandleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
