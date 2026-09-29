package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// createTenantForAuth creates a tenant through the API and returns its key.
func createTenantForAuth(t *testing.T, router http.Handler, id string) string {
	t.Helper()
	b, err := json.Marshal(CreateTenantRequest{Id: strPtr(id), TeamName: strPtr("Team " + id)})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create tenant: %d %s", w.Code, w.Body.String())
	}
	var resp CreateTenantResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	return resp.ApiKey
}

func decodeError(t *testing.T, w *httptest.ResponseRecorder) ErrorResponse {
	t.Helper()
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected JSON error response, got Content-Type %q", ct)
	}
	var e ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&e); err != nil {
		t.Fatalf("decode error body %q: %v", w.Body.String(), err)
	}
	return e
}

func TestValidatorRejectsMissingRequiredField(t *testing.T) {
	router, _ := setupTestRouter(t)
	key := createTenantForAuth(t, router, "t1")

	// CreateFilterRequest requires "pattern".
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/t1/filters", strings.NewReader(`{"msg_suffix":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w)
	if !strings.Contains(e.Error, "pattern") {
		t.Errorf("expected error to name the missing field, got %q", e.Error)
	}
}

func TestValidatorRejectsMalformedJSON(t *testing.T) {
	router, _ := setupTestRouter(t)
	key := createTenantForAuth(t, router, "t1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/t1/filters", strings.NewReader(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	decodeError(t, w)
}

func TestValidatorAuthRunsBeforeBodyValidation(t *testing.T) {
	router, _ := setupTestRouter(t)
	createTenantForAuth(t, router, "t1")

	// Invalid body AND no credentials: caller must not learn about the
	// schema without authenticating first.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/t1/filters", strings.NewReader(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantKeyCannotAccessOtherTenant(t *testing.T) {
	router, _ := setupTestRouter(t)
	key1 := createTenantForAuth(t, router, "t1")
	createTenantForAuth(t, router, "t2")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/t2", nil)
	req.Header.Set("Authorization", "Bearer "+key1)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if e := decodeError(t, w); e.Error != "forbidden" {
		t.Errorf("error = %q, want forbidden", e.Error)
	}
}

func TestAdminKeyDoesNotGrantTenantAccess(t *testing.T) {
	router, _ := setupTestRouter(t)
	createTenantForAuth(t, router, "t1")

	// Tenant-scoped operations declare only TenantKey in the spec, so the
	// admin key is just a wrong key here.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/t1", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBearerSchemeIsCaseInsensitive(t *testing.T) {
	router, _ := setupTestRouter(t)
	key := createTenantForAuth(t, router, "t1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/t1", nil)
	req.Header.Set("Authorization", "bearer "+key)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWebhookSkipsValidation(t *testing.T) {
	router, _ := setupTestRouter(t)

	// The validator would reject a request whose Content-Type is not in the
	// spec; the webhook must keep accepting it so Zoom always gets a 200.
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", strings.NewReader(`{"event":"meeting.unknown"}`))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWebhookCRCStillWorks(t *testing.T) {
	router, _ := setupTestRouter(t)

	body := `{"event":"endpoint.url_validation","payload":{"plainToken":"abc123"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		PlainToken     string `json:"plainToken"`
		EncryptedToken string `json:"encryptedToken"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.PlainToken != "abc123" || resp.EncryptedToken == "" {
		t.Errorf("unexpected CRC response: %+v", resp)
	}
}
