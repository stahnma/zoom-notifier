package zoom

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("expected Authorization header")
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("expected form content-type, got %s", r.Header.Get("Content-Type"))
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-token-123",
			"token_type":   "bearer",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, server.URL, "client-id", "client-secret", "account-id")
	token, err := client.GetAccessToken()
	if err != nil {
		t.Fatalf("get access token: %v", err)
	}
	if token != "test-token-123" {
		t.Errorf("expected 'test-token-123', got '%s'", token)
	}
}

func TestGetAccessToken_Cached(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "cached-token",
			"token_type":   "bearer",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, server.URL, "id", "secret", "acct")

	// First call fetches
	token1, err := client.GetAccessToken()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Second call should use cache
	token2, err := client.GetAccessToken()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if token1 != token2 {
		t.Error("tokens should be identical (cached)")
	}
	if callCount != 1 {
		t.Errorf("expected 1 API call (cached), got %d", callCount)
	}
}

func TestGetAccessToken_CacheExpiry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "token-" + time.Now().String(),
			"token_type":   "bearer",
			"expires_in":   1, // 1 second expiry
		})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, server.URL, "id", "secret", "acct")

	_, _ = client.GetAccessToken()
	// Manually expire the token
	client.tokenExpiry = time.Now().Add(-time.Minute)
	_, _ = client.GetAccessToken()

	if callCount != 2 {
		t.Errorf("expected 2 API calls (cache expired), got %d", callCount)
	}
}

func TestGetMeetingJoinLink(t *testing.T) {
	oauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-token",
			"token_type":   "bearer",
			"expires_in":   3600,
		})
	}))
	defer oauthServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"join_url": "https://zoom.us/j/12345?pwd=abc123",
		})
	}))
	defer apiServer.Close()

	client := NewAPIClient(oauthServer.URL, apiServer.URL, "id", "secret", "acct")
	link, err := client.GetMeetingJoinLink("12345")
	if err != nil {
		t.Fatalf("get meeting join link: %v", err)
	}
	if link != "https://zoom.us/j/12345?pwd=abc123" {
		t.Errorf("unexpected link: %s", link)
	}
}

func TestGetMeetingJoinLink_APIError(t *testing.T) {
	oauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-token",
			"token_type":   "bearer",
			"expires_in":   3600,
		})
	}))
	defer oauthServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer apiServer.Close()

	client := NewAPIClient(oauthServer.URL, apiServer.URL, "id", "secret", "acct")
	_, err := client.GetMeetingJoinLink("nonexistent")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}
