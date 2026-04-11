package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecovery_NormalHandler(t *testing.T) {
	handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected 'ok', got '%s'", w.Body.String())
	}
}

func TestRecovery_PanicHandler(t *testing.T) {
	handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went wrong")
	}))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()

	// Should not panic — recovery middleware catches it
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRecovery_ErrAbortHandler_RePanics(t *testing.T) {
	handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	req := httptest.NewRequest(http.MethodGet, "/panic-error", nil)
	w := httptest.NewRecorder()

	// Should re-panic with http.ErrAbortHandler
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected re-panic for http.ErrAbortHandler")
		}
		if r != http.ErrAbortHandler {
			t.Errorf("expected http.ErrAbortHandler, got %v", r)
		}
	}()

	handler.ServeHTTP(w, req)
	t.Fatal("should not reach here")
}

func TestRecovery_PanicWithNil(t *testing.T) {
	handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(nil)
	}))

	req := httptest.NewRequest(http.MethodGet, "/panic-nil", nil)
	w := httptest.NewRecorder()

	// Go 1.21+ converts panic(nil) to a runtime.PanicNilError, which
	// recover() catches. Recovery middleware returns 500.
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
