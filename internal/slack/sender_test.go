package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestSender_Send(t *testing.T) {
	var mu sync.Mutex
	var receivedCalls []map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}

		mu.Lock()
		receivedCalls = append(receivedCalls, map[string]interface{}{
			"token":   r.FormValue("token"),
			"channel": r.FormValue("channel"),
		})
		mu.Unlock()

		// Slack API response format
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"channel": "C123",
			"ts":      "1234567890.123456",
		})
	}))
	defer server.Close()

	sender := NewSender(server.URL + "/")
	ctx := context.Background()

	err := sender.Send(ctx, "xoxb-test-token", "#general", "Alice has joined the standup.")
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(receivedCalls) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(receivedCalls))
	}
	if receivedCalls[0]["channel"] != "#general" {
		t.Errorf("expected channel '#general', got '%s'", receivedCalls[0]["channel"])
	}
}

func TestSender_SendError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "channel_not_found",
		})
	}))
	defer server.Close()

	sender := NewSender(server.URL + "/")
	ctx := context.Background()

	err := sender.Send(ctx, "xoxb-test-token", "#nonexistent", "test message")
	if err == nil {
		t.Error("expected error for failed API call")
	}
}

func TestSender_MultipleChannels(t *testing.T) {
	var mu sync.Mutex
	var channels []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		mu.Lock()
		channels = append(channels, r.FormValue("channel"))
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"channel": "C123",
			"ts":      "1234567890.123456",
		})
	}))
	defer server.Close()

	sender := NewSender(server.URL + "/")
	ctx := context.Background()

	sender.Send(ctx, "xoxb-token", "#general", "msg 1")
	sender.Send(ctx, "xoxb-token", "#dev", "msg 2")

	mu.Lock()
	defer mu.Unlock()
	if len(channels) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(channels))
	}
}
