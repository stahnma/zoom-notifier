package slack

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

const testSigningSecret = "test-signing-secret"

func signSlackRequest(t *testing.T, body string, secret string, ts string) http.Header {
	t.Helper()
	baseString := fmt.Sprintf("v0:%s:%s", ts, body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(baseString))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))
	h := http.Header{}
	h.Set("X-Slack-Request-Timestamp", ts)
	h.Set("X-Slack-Signature", sig)
	h.Set("Content-Type", "application/x-www-form-urlencoded")
	return h
}

func setupHTTPCommandHandler(t *testing.T) *HTTPCommandHandler {
	t.Helper()
	s := setupOAuthTestStore(t)
	tenant := &store.Tenant{ID: "T-HTTP", TeamName: "HTTP Test", APIKey: "key", InstalledAt: time.Now()}
	if err := s.CreateTenant(context.Background(), tenant); err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}
	cmdHandler := NewCommandHandler(s)
	return NewHTTPCommandHandler(cmdHandler, testSigningSecret)
}

func TestHTTPCommandHandler_ValidRequest(t *testing.T) {
	httpHandler := setupHTTPCommandHandler(t)

	body := "team_id=T-HTTP&user_id=U123&channel_id=C123&text=help"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	headers := signSlackRequest(t, body, testSigningSecret, ts)

	req := httptest.NewRequest(http.MethodPost, "/slack/commands", strings.NewReader(body))
	req.Header = headers
	w := httptest.NewRecorder()

	httpHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["response_type"] != "ephemeral" {
		t.Errorf("expected response_type=ephemeral, got %s", resp["response_type"])
	}

	if !strings.Contains(resp["text"], "zoom-notifier commands") {
		t.Errorf("expected help text in response, got: %s", resp["text"])
	}
}

func TestHTTPCommandHandler_InvalidSignature(t *testing.T) {
	httpHandler := setupHTTPCommandHandler(t)

	body := "team_id=T-HTTP&user_id=U123&channel_id=C123&text=help"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	// Sign with wrong secret
	headers := signSlackRequest(t, body, "wrong-secret", ts)

	req := httptest.NewRequest(http.MethodPost, "/slack/commands", strings.NewReader(body))
	req.Header = headers
	w := httptest.NewRecorder()

	httpHandler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHTTPCommandHandler_ExpiredTimestamp(t *testing.T) {
	httpHandler := setupHTTPCommandHandler(t)

	body := "team_id=T-HTTP&user_id=U123&channel_id=C123&text=help"
	// Timestamp 10 minutes ago
	ts := strconv.FormatInt(time.Now().Unix()-600, 10)
	headers := signSlackRequest(t, body, testSigningSecret, ts)

	req := httptest.NewRequest(http.MethodPost, "/slack/commands", strings.NewReader(body))
	req.Header = headers
	w := httptest.NewRecorder()

	httpHandler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHTTPCommandHandler_MissingHeaders(t *testing.T) {
	httpHandler := setupHTTPCommandHandler(t)

	body := "team_id=T-HTTP&user_id=U123&channel_id=C123&text=help"

	req := httptest.NewRequest(http.MethodPost, "/slack/commands", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	httpHandler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
