package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store/sqlite"
)

const testAdminKey = "test-admin-key-12345"

func setupTestRouter(t *testing.T) (http.Handler, *sqlite.SQLiteStore) {
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

	server := NewServer(s, "test-version", "abc123", "2026-01-01", nil, "test-webhook-secret")
	router := SetupRouter(server, s, testAdminKey)
	return router, s
}

func TestHealthCheck(t *testing.T) {
	router, _ := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}
	if resp.Version != "test-version" {
		t.Errorf("expected version test-version, got %s", resp.Version)
	}
}

func TestCreateTenantWithAdminKey(t *testing.T) {
	router, _ := setupTestRouter(t)

	body := CreateTenantRequest{TeamName: strPtr("Test Team")}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp CreateTenantResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Id == "" {
		t.Error("expected non-empty tenant ID")
	}
	if resp.ApiKey == "" {
		t.Error("expected non-empty API key")
	}
}

func TestCreateTenantWithoutKey(t *testing.T) {
	router, _ := setupTestRouter(t)

	body := CreateTenantRequest{TeamName: strPtr("Test Team")}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTenantWithWrongKey(t *testing.T) {
	router, _ := setupTestRouter(t)

	body := CreateTenantRequest{TeamName: strPtr("Test Team")}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetTenantWithTenantKey(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Create a tenant first
	createBody := CreateTenantRequest{Id: strPtr("t1"), TeamName: strPtr("Team1")}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", w.Code, w.Body.String())
	}

	var createResp CreateTenantResponse
	json.NewDecoder(w.Body).Decode(&createResp)

	// Get tenant with the tenant's API key
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/t1", nil)
	req.Header.Set("Authorization", "Bearer "+createResp.ApiKey)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tenant Tenant
	json.NewDecoder(w.Body).Decode(&tenant)
	if tenant.Id != "t1" {
		t.Errorf("expected tenant id t1, got %s", tenant.Id)
	}
}

func TestGetTenantWithWrongKey(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Create a tenant first
	createBody := CreateTenantRequest{Id: strPtr("t2"), TeamName: strPtr("Team2")}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", w.Code)
	}

	// Get tenant with wrong key
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/t2", nil)
	req.Header.Set("Authorization", "Bearer wrong-api-key")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetTenantNotFound(t *testing.T) {
	router, _ := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer some-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Tenant doesn't exist, so TenantKeyMiddleware returns 401
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListTenantsWithAdminKey(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Create two tenants
	for _, name := range []string{"Team A", "Team B"} {
		body := CreateTenantRequest{TeamName: strPtr(name)}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testAdminKey)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create failed: %d", w.Code)
		}
	}

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tenants []Tenant
	json.NewDecoder(w.Body).Decode(&tenants)
	if len(tenants) != 2 {
		t.Errorf("expected 2 tenants, got %d", len(tenants))
	}
}

func TestDeleteTenantWithTenantKey(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Create
	body := CreateTenantRequest{Id: strPtr("del-me")}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", w.Code)
	}
	var createResp CreateTenantResponse
	json.NewDecoder(w.Body).Decode(&createResp)

	// Delete with tenant's own API key
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/del-me", nil)
	req.Header.Set("Authorization", "Bearer "+createResp.ApiKey)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubscriptionCRUD(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Create tenant
	createBody := CreateTenantRequest{Id: strPtr("sub-tenant")}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var createResp CreateTenantResponse
	json.NewDecoder(w.Body).Decode(&createResp)
	tenantKey := createResp.ApiKey
	auth := "Bearer " + tenantKey

	// Create subscription
	subBody := CreateSubscriptionRequest{
		Type:   CreateSubscriptionRequestTypeSlack,
		Target: "#general",
	}
	b, _ = json.Marshal(subBody)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tenants/sub-tenant/subscriptions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create subscription: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var sub Subscription
	json.NewDecoder(w.Body).Decode(&sub)
	if sub.Target != "#general" {
		t.Errorf("expected target #general, got %s", sub.Target)
	}
	if sub.Type != SubscriptionTypeSlack {
		t.Errorf("expected type slack, got %s", sub.Type)
	}

	// List subscriptions
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/sub-tenant/subscriptions", nil)
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list subscriptions: expected 200, got %d", w.Code)
	}

	var subs []Subscription
	json.NewDecoder(w.Body).Decode(&subs)
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}

	// Update subscription
	updateBody := UpdateSubscriptionRequest{
		Enabled: boolPtr(false),
		Target:  strPtr("#alerts"),
	}
	b, _ = json.Marshal(updateBody)
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/tenants/sub-tenant/subscriptions/"+intToStr(sub.Id), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update subscription: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated Subscription
	json.NewDecoder(w.Body).Decode(&updated)
	if updated.Target != "#alerts" {
		t.Errorf("expected target #alerts, got %s", updated.Target)
	}
	if updated.Enabled {
		t.Error("expected subscription to be disabled")
	}

	// Delete subscription
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/sub-tenant/subscriptions/"+intToStr(sub.Id), nil)
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("delete subscription: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFilterCRUD(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Create tenant
	createBody := CreateTenantRequest{Id: strPtr("filter-tenant")}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var createResp CreateTenantResponse
	json.NewDecoder(w.Body).Decode(&createResp)
	auth := "Bearer " + createResp.ApiKey

	// Create filter
	filterBody := CreateFilterRequest{Pattern: "standup"}
	b, _ = json.Marshal(filterBody)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tenants/filter-tenant/filters", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create filter: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var filter MeetingFilter
	json.NewDecoder(w.Body).Decode(&filter)
	if filter.Pattern != "standup" {
		t.Errorf("expected pattern standup, got %s", filter.Pattern)
	}

	// List filters
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/filter-tenant/filters", nil)
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list filters: expected 200, got %d", w.Code)
	}

	var filters []MeetingFilter
	json.NewDecoder(w.Body).Decode(&filters)
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}

	// Delete filter
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/filter-tenant/filters/"+intToStr(filter.Id), nil)
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("delete filter: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIRCConfigPutAndGet(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Create tenant
	createBody := CreateTenantRequest{Id: strPtr("irc-tenant")}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var createResp CreateTenantResponse
	json.NewDecoder(w.Body).Decode(&createResp)
	auth := "Bearer " + createResp.ApiKey

	// PUT IRC config
	ircBody := IRCConfigRequest{
		Server:   "irc.libera.chat:6697",
		Nick:     "zoombot",
		Password: "secret",
		UseTls:   boolPtr(true),
	}
	b, _ = json.Marshal(ircBody)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/tenants/irc-tenant/irc", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("put IRC: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// GET IRC config
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/irc-tenant/irc", nil)
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get IRC: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var configs []IRCConfig
	json.NewDecoder(w.Body).Decode(&configs)
	if len(configs) != 1 {
		t.Fatalf("expected 1 IRC config, got %d", len(configs))
	}
	if configs[0].Server != "irc.libera.chat:6697" {
		t.Errorf("expected server irc.libera.chat:6697, got %s", configs[0].Server)
	}
	if !configs[0].UseTls {
		t.Error("expected use_tls to be true")
	}
}

func TestAdminCRUD(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Create tenant
	createBody := CreateTenantRequest{Id: strPtr("admin-tenant")}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var createResp CreateTenantResponse
	json.NewDecoder(w.Body).Decode(&createResp)
	auth := "Bearer " + createResp.ApiKey

	// Add admin
	adminBody := AddAdminRequest{SlackUserId: "U12345"}
	b, _ = json.Marshal(adminBody)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tenants/admin-tenant/admins", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("add admin: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// List admins
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/admin-tenant/admins", nil)
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list admins: expected 200, got %d", w.Code)
	}

	var admins []TenantAdmin
	json.NewDecoder(w.Body).Decode(&admins)
	if len(admins) != 1 {
		t.Fatalf("expected 1 admin, got %d", len(admins))
	}
	if admins[0].SlackUserId != "U12345" {
		t.Errorf("expected slack user id U12345, got %s", admins[0].SlackUserId)
	}

	// Remove admin
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/admin-tenant/admins/U12345", nil)
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("remove admin: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRotateAPIKey(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Create tenant
	createBody := CreateTenantRequest{Id: strPtr("rotate-tenant")}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var createResp CreateTenantResponse
	json.NewDecoder(w.Body).Decode(&createResp)
	oldKey := createResp.ApiKey

	// Rotate key
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tenants/rotate-tenant/rotate-key", nil)
	req.Header.Set("Authorization", "Bearer "+oldKey)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("rotate key: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var rotateResp RotateKeyResponse
	json.NewDecoder(w.Body).Decode(&rotateResp)
	if rotateResp.ApiKey == "" {
		t.Error("expected non-empty new API key")
	}
	if rotateResp.ApiKey == oldKey {
		t.Error("expected new key to be different from old key")
	}

	// Old key should no longer work
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/rotate-tenant", nil)
	req.Header.Set("Authorization", "Bearer "+oldKey)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("old key: expected 403, got %d", w.Code)
	}

	// New key should work
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/rotate-tenant", nil)
	req.Header.Set("Authorization", "Bearer "+rotateResp.ApiKey)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("new key: expected 200, got %d", w.Code)
	}
}

func TestTenantDefaultsInResponse(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Create tenant
	createBody := CreateTenantRequest{Id: strPtr("defaults-tenant"), TeamName: strPtr("Defaults Team")}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", w.Code, w.Body.String())
	}
	var createResp CreateTenantResponse
	json.NewDecoder(w.Body).Decode(&createResp)

	// Get tenant and check defaults are present
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/defaults-tenant", nil)
	req.Header.Set("Authorization", "Bearer "+createResp.ApiKey)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tenant Tenant
	json.NewDecoder(w.Body).Decode(&tenant)
	if tenant.DefaultMsgSuffix == nil {
		t.Error("expected default_msg_suffix to be present")
	}
	if tenant.DefaultIncludeLink == nil {
		t.Error("expected default_include_link to be present")
	}
}

func TestPutTenantDefaults(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Create tenant
	createBody := CreateTenantRequest{Id: strPtr("put-defaults-tenant")}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var createResp CreateTenantResponse
	json.NewDecoder(w.Body).Decode(&createResp)
	auth := "Bearer " + createResp.ApiKey

	// Update defaults
	defaultsBody := TenantDefaultsRequest{
		DefaultMsgSuffix:   strPtr("a custom suffix"),
		DefaultIncludeLink: boolPtr(false),
	}
	b, _ = json.Marshal(defaultsBody)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/tenants/put-defaults-tenant/defaults", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("put defaults: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tenant Tenant
	json.NewDecoder(w.Body).Decode(&tenant)
	if tenant.DefaultMsgSuffix == nil || *tenant.DefaultMsgSuffix != "a custom suffix" {
		t.Errorf("expected default_msg_suffix 'a custom suffix', got %v", tenant.DefaultMsgSuffix)
	}
	if tenant.DefaultIncludeLink == nil || *tenant.DefaultIncludeLink != false {
		t.Errorf("expected default_include_link false, got %v", tenant.DefaultIncludeLink)
	}
}

func TestFilterWithOverrides(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Create tenant
	createBody := CreateTenantRequest{Id: strPtr("filter-override-tenant")}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var createResp CreateTenantResponse
	json.NewDecoder(w.Body).Decode(&createResp)
	auth := "Bearer " + createResp.ApiKey

	// Create filter with overrides
	filterBody := CreateFilterRequest{
		Pattern:     "standup",
		MsgSuffix:   strPtr("the standup."),
		IncludeLink: boolPtr(false),
	}
	b, _ = json.Marshal(filterBody)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tenants/filter-override-tenant/filters", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create filter: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var filter MeetingFilter
	json.NewDecoder(w.Body).Decode(&filter)
	if filter.MsgSuffix == nil || *filter.MsgSuffix != "the standup." {
		t.Errorf("expected msg_suffix 'the standup.', got %v", filter.MsgSuffix)
	}
	if filter.IncludeLink == nil || *filter.IncludeLink != false {
		t.Errorf("expected include_link false, got %v", filter.IncludeLink)
	}

	// List filters and verify overrides present
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/filter-override-tenant/filters", nil)
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var filters []MeetingFilter
	json.NewDecoder(w.Body).Decode(&filters)
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].MsgSuffix == nil || *filters[0].MsgSuffix != "the standup." {
		t.Errorf("list: expected msg_suffix 'the standup.', got %v", filters[0].MsgSuffix)
	}

	// Update filter
	updateBody := UpdateFilterRequest{
		MsgSuffix: strPtr("updated suffix"),
	}
	b, _ = json.Marshal(updateBody)
	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/tenants/filter-override-tenant/filters/%d", filter.Id), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update filter: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updatedFilter MeetingFilter
	json.NewDecoder(w.Body).Decode(&updatedFilter)
	if updatedFilter.MsgSuffix == nil || *updatedFilter.MsgSuffix != "updated suffix" {
		t.Errorf("expected msg_suffix 'updated suffix', got %v", updatedFilter.MsgSuffix)
	}
}

func TestZoomCredentialsPut(t *testing.T) {
	router, _ := setupTestRouter(t)

	// Create tenant
	createBody := CreateTenantRequest{Id: strPtr("zoom-tenant")}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var createResp CreateTenantResponse
	json.NewDecoder(w.Body).Decode(&createResp)
	auth := "Bearer " + createResp.ApiKey

	// PUT zoom credentials
	zoomBody := ZoomCredentialsRequest{
		ClientId:     "client123",
		ClientSecret: "secret456",
		AccountId:    "acct789",
	}
	b, _ = json.Marshal(zoomBody)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/tenants/zoom-tenant/zoom", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("put zoom creds: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Helpers ---

func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func intToStr(i int64) string {
	return fmt.Sprintf("%d", i)
}
