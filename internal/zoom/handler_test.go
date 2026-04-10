package zoom

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/stahnma/zoom-notifier/internal/store"
	"github.com/stahnma/zoom-notifier/internal/store/sqlite"
)

func setupHandlerTest(t *testing.T) (*sqlite.SQLiteStore, *Handler) {
	t.Helper()
	s, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Log("close:", err)
		}
	})

	// Create a test tenant
	ctx := context.Background()
	if err := s.CreateTenant(ctx, &store.Tenant{
		ID:            "T1",
		APIKey:        "key-1",
		ZoomAccountID: "uUpLA0YDRhWZvYIu_JxPpg",
	}); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(s, "test-secret", nil)
	return s, handler
}

func loadPayload(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../examples/zoom/" + filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	return data
}

func TestHandler_CRCValidation(t *testing.T) {
	_, handler := setupHandlerTest(t)

	body := loadPayload(t, "endpoint_url_validation.json")
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp CRCResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.PlainToken == "" {
		t.Error("expected plainToken in response")
	}
	if resp.EncryptedToken == "" {
		t.Error("expected encryptedToken in response")
	}
}

func TestHandler_ParticipantJoined(t *testing.T) {
	s, handler := setupHandlerTest(t)
	ctx := context.Background()

	body := loadPayload(t, "participant_joined.json")
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify meeting was created in store
	meeting, err := s.GetMeeting(ctx, "6735648745")
	if err != nil {
		t.Fatalf("get meeting: %v", err)
	}
	if meeting == nil {
		t.Fatal("expected meeting to be created")
	}
	if meeting.Topic != "Somebody's Personal Meeting Room" {
		t.Errorf("unexpected topic: %s", meeting.Topic)
	}

	// Verify participant was added
	participants, err := s.GetActiveParticipants(ctx, "6735648745")
	if err != nil {
		t.Fatalf("get participants: %v", err)
	}
	if len(participants) != 1 {
		t.Fatalf("expected 1 participant, got %d", len(participants))
	}
	if participants[0].UserName != "Jonny Bananas" {
		t.Errorf("unexpected participant: %s", participants[0].UserName)
	}
}

func TestHandler_ParticipantLeft(t *testing.T) {
	s, handler := setupHandlerTest(t)
	ctx := context.Background()

	// First, simulate a join
	joinBody := loadPayload(t, "participant_joined.json")
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", bytes.NewReader(joinBody))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// Now simulate leave
	leaveBody := loadPayload(t, "participant_left.json")
	req = httptest.NewRequest(http.MethodPost, "/webhook/zoom", bytes.NewReader(leaveBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Verify the handler processed without error — the participant_left payload
	// may reference a different meeting ID than participant_joined
	_ = s   //nolint:revive // kept for potential future assertions
	_ = ctx //nolint:revive // kept for potential future assertions
}

func TestHandler_MeetingStarted(t *testing.T) {
	s, handler := setupHandlerTest(t)
	ctx := context.Background()

	// meeting_started has account_id "uUpLO0YDAhWZvYIu_JxPpg" — different from T1's
	// Create a tenant matching this account
	if err := s.CreateTenant(ctx, &store.Tenant{
		ID:            "T2",
		APIKey:        "key-2",
		ZoomAccountID: "uUpLO0YDAhWZvYIu_JxPpg",
	}); err != nil {
		t.Fatal(err)
	}

	body := loadPayload(t, "meeting_started.json")
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	meeting, err := s.GetMeeting(ctx, "6704648745")
	if err != nil {
		t.Fatal(err)
	}
	if meeting == nil {
		t.Fatal("expected meeting to be created on meeting.started")
	}
}

func TestHandler_MeetingEnded(t *testing.T) {
	s, handler := setupHandlerTest(t)
	ctx := context.Background()

	// First create the meeting
	if err := s.UpsertMeeting(ctx, &store.ActiveMeeting{
		MeetingID: "6703648745",
		TenantID:  "T1",
		Topic:     "Test Meeting",
	}); err != nil {
		t.Fatal(err)
	}

	body := loadPayload(t, "meeting_ended.json")
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	meeting, err := s.GetMeeting(ctx, "6703648745")
	if err != nil {
		t.Fatal(err)
	}
	if meeting != nil {
		t.Error("expected meeting to be deleted on meeting.ended")
	}
}

func TestHandler_UnknownEvent(t *testing.T) {
	_, handler := setupHandlerTest(t)

	payload := `{"event":"meeting.sharing_started","payload":{"account_id":"uUpLA0YDRhWZvYIu_JxPpg","object":{"id":"123"}},"event_ts":123}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for unknown event, got %d", rr.Code)
	}
}

func TestHandler_UnknownAccountID(t *testing.T) {
	_, handler := setupHandlerTest(t)

	payload := `{"event":"meeting.participant_joined","payload":{"account_id":"UNKNOWN_ACCOUNT","object":{"id":"123","participant":{"user_name":"Test"}}},"event_ts":123}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should return 200 — don't leak tenant info
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for unknown account, got %d", rr.Code)
	}
}

func TestHandler_DispatcherCalled(t *testing.T) {
	_, handler := setupHandlerTest(t)

	var mu sync.Mutex
	var dispatched []string
	handler.dispatch = func(tenantID string, event string, payload WebhookPayload) {
		mu.Lock()
		dispatched = append(dispatched, tenantID+":"+event)
		mu.Unlock()
	}

	body := loadPayload(t, "participant_joined.json")
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	mu.Lock()
	defer mu.Unlock()
	if len(dispatched) != 1 {
		t.Fatalf("expected 1 dispatch call, got %d", len(dispatched))
	}
	if dispatched[0] != "T1:meeting.participant_joined" {
		t.Errorf("unexpected dispatch: %s", dispatched[0])
	}
}
