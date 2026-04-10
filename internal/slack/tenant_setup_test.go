package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

func setupTenantSetupTest(t *testing.T) (*TenantSetupHandler, store.Store) {
	t.Helper()
	s := setupOAuthTestStore(t)

	tenant := &store.Tenant{
		ID:          "T-SETUP",
		TeamName:    "Setup Test Workspace",
		APIKey:      "valid-api-key",
		InstalledAt: time.Now(),
	}
	if err := s.CreateTenant(context.Background(), tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	handler := NewTenantSetupHandler(s)
	return handler, s
}

func TestTenantSetupGETValidKey(t *testing.T) {
	handler, _ := setupTenantSetupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/tenant/setup?key=valid-api-key", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Zoom Account ID") {
		t.Error("expected page to contain 'Zoom Account ID'")
	}
	if !strings.Contains(body, "Setup Test Workspace") {
		t.Error("expected page to contain workspace name")
	}
}

func TestTenantSetupGETInvalidKey(t *testing.T) {
	handler, _ := setupTenantSetupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/tenant/setup?key=bad-key", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantSetupGETMissingKey(t *testing.T) {
	handler, _ := setupTenantSetupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/tenant/setup", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestTenantSetupPOSTValidAccountID(t *testing.T) {
	handler, s := setupTenantSetupTest(t)

	form := url.Values{
		"zoom_account_id": {"abc123"},
	}
	req := httptest.NewRequest(http.MethodPost, "/tenant/setup?key=valid-api-key", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "saved successfully") {
		t.Error("expected success message in response")
	}

	// Verify tenant was updated
	tenant, err := s.GetTenant(context.Background(), "T-SETUP")
	if err != nil {
		t.Fatal(err)
	}
	if tenant.ZoomAccountID != "abc123" {
		t.Errorf("expected ZoomAccountID 'abc123', got '%s'", tenant.ZoomAccountID)
	}
}

func TestTenantSetupPOSTWithCredentials(t *testing.T) {
	handler, s := setupTenantSetupTest(t)

	form := url.Values{
		"zoom_account_id":    {"abc123"},
		"zoom_client_id":     {"client-id-val"},
		"zoom_client_secret": {"client-secret-val"},
	}
	req := httptest.NewRequest(http.MethodPost, "/tenant/setup?key=valid-api-key", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify zoom credentials were saved
	creds, err := s.GetZoomCredentials(context.Background(), "T-SETUP")
	if err != nil {
		t.Fatal(err)
	}
	if creds == nil {
		t.Fatal("expected zoom credentials to be saved")
	}
	if creds.ClientID != "client-id-val" {
		t.Errorf("expected client ID 'client-id-val', got '%s'", creds.ClientID)
	}
	if creds.ClientSecret != "client-secret-val" {
		t.Errorf("expected client secret 'client-secret-val', got '%s'", creds.ClientSecret)
	}
}

func TestTenantSetupPOSTMissingAccountID(t *testing.T) {
	handler, _ := setupTenantSetupTest(t)

	form := url.Values{
		"zoom_account_id": {""},
	}
	req := httptest.NewRequest(http.MethodPost, "/tenant/setup?key=valid-api-key", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "required") {
		t.Error("expected error about required account ID")
	}
}
