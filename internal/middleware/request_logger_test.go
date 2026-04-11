package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestRequestLogger_LogsRequestFields(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})
	defer log.SetOutput(os.Stderr)

	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	output := buf.String()
	if !strings.Contains(output, "method=GET") {
		t.Errorf("expected log to contain method=GET, got: %s", output)
	}
	if !strings.Contains(output, "path=/healthz") {
		t.Errorf("expected log to contain path=/healthz, got: %s", output)
	}
	if !strings.Contains(output, "status=200") {
		t.Errorf("expected log to contain status=200, got: %s", output)
	}
	if !strings.Contains(output, "duration=") {
		t.Errorf("expected log to contain duration=, got: %s", output)
	}
}

func TestRequestLogger_CapturesStatusCode(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})
	defer log.SetOutput(os.Stderr)

	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/missing", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	output := buf.String()
	if !strings.Contains(output, "status=404") {
		t.Errorf("expected log to contain status=404, got: %s", output)
	}
	if !strings.Contains(output, "method=POST") {
		t.Errorf("expected log to contain method=POST, got: %s", output)
	}
}

func TestRequestLogger_DefaultsTo200(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})
	defer log.SetOutput(os.Stderr)

	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No explicit WriteHeader — Go defaults to 200 on first Write
		_, _ = w.Write([]byte("hello"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	output := buf.String()
	if !strings.Contains(output, "status=200") {
		t.Errorf("expected log to contain status=200, got: %s", output)
	}
}

func TestRequestLogger_PassesResponseThrough(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "value")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/resource", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	if w.Body.String() != "created" {
		t.Errorf("expected body 'created', got '%s'", w.Body.String())
	}
	if w.Header().Get("X-Custom") != "value" {
		t.Errorf("expected X-Custom header, got '%s'", w.Header().Get("X-Custom"))
	}
}

func TestStatusRecorder_Unwrap(t *testing.T) {
	w := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

	unwrapped := rec.Unwrap()
	if unwrapped != w {
		t.Error("expected Unwrap to return the underlying ResponseWriter")
	}
}
