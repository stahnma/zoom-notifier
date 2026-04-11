package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stahnma/zoom-notifier/internal/store"
)

func TestLivezHandler(t *testing.T) {
	handler := LivezHandler()
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
}

func TestReadyzHandler_Healthy(t *testing.T) {
	s := &mockProbeStore{healthy: true}
	handler := ReadyzHandler(s)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
	if resp["database"] != "ok" {
		t.Errorf("expected database ok, got %s", resp["database"])
	}
}

func TestReadyzHandler_Unhealthy(t *testing.T) {
	s := &mockProbeStore{healthy: false}
	handler := ReadyzHandler(s)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "not ready" {
		t.Errorf("expected status 'not ready', got %s", resp["status"])
	}
}

// mockProbeStore is a minimal store mock for probe tests.
type mockProbeStore struct {
	store.Store
	healthy bool
}

func (m *mockProbeStore) ListTenants(_ context.Context) ([]*store.Tenant, error) {
	if !m.healthy {
		return nil, context.DeadlineExceeded
	}
	return []*store.Tenant{}, nil
}
