package slack

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func setupInteractionHandler(t *testing.T) *InteractionHandler {
	t.Helper()
	s := setupOAuthTestStore(t)
	return NewInteractionHandler(s, testSigningSecret)
}

func makeInteractionBody(t *testing.T, payload InteractionPayload) string {
	t.Helper()
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	return "payload=" + url.QueryEscape(string(payloadJSON))
}

func TestInteractionHandler_InvalidSignature(t *testing.T) {
	handler := setupInteractionHandler(t)

	payload := InteractionPayload{Type: "view_submission"}
	body := makeInteractionBody(t, payload)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	headers := signSlackRequest(t, body, "wrong-secret", ts)

	req := httptest.NewRequest(http.MethodPost, "/slack/interactions", strings.NewReader(body))
	req.Header = headers
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestInteractionHandler_ValidPayloadWithRegisteredCallback(t *testing.T) {
	handler := setupInteractionHandler(t)

	called := false
	handler.RegisterHandler("test_callback", func(p InteractionPayload) error {
		called = true
		if p.User.ID != "U123" {
			t.Errorf("expected user ID U123, got %s", p.User.ID)
		}
		return nil
	})

	payload := InteractionPayload{Type: "view_submission"}
	payload.User.ID = "U123"
	payload.User.TeamID = "T123"
	payload.View.CallbackID = "test_callback"

	body := makeInteractionBody(t, payload)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	headers := signSlackRequest(t, body, testSigningSecret, ts)

	req := httptest.NewRequest(http.MethodPost, "/slack/interactions", strings.NewReader(body))
	req.Header = headers
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestInteractionHandler_UnknownCallbackReturns200(t *testing.T) {
	handler := setupInteractionHandler(t)

	payload := InteractionPayload{Type: "view_submission"}
	payload.View.CallbackID = "unknown_callback"

	body := makeInteractionBody(t, payload)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	headers := signSlackRequest(t, body, testSigningSecret, ts)

	req := httptest.NewRequest(http.MethodPost, "/slack/interactions", strings.NewReader(body))
	req.Header = headers
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInteractionHandler_MalformedPayload(t *testing.T) {
	handler := setupInteractionHandler(t)

	body := "payload=" + url.QueryEscape("not valid json{{{")
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	headers := signSlackRequest(t, body, testSigningSecret, ts)

	req := httptest.NewRequest(http.MethodPost, "/slack/interactions", strings.NewReader(body))
	req.Header = headers
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInteractionHandler_HandlerError(t *testing.T) {
	handler := setupInteractionHandler(t)

	handler.RegisterHandler("failing_callback", func(p InteractionPayload) error {
		return fmt.Errorf("something went wrong")
	})

	payload := InteractionPayload{Type: "view_submission"}
	payload.View.CallbackID = "failing_callback"

	body := makeInteractionBody(t, payload)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	headers := signSlackRequest(t, body, testSigningSecret, ts)

	req := httptest.NewRequest(http.MethodPost, "/slack/interactions", strings.NewReader(body))
	req.Header = headers
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}
