package zoom

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
		ZoomAccountID: testAccount,
	}); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(s, testSecret, nil)
	return s, handler
}

const (
	testSecret  = "test-secret"
	testAccount = "uUpLA0YDRhWZvYIu_JxPpg" // account_id in most examples/zoom payloads
)

// signedRequest builds a webhook request signed the way Zoom signs it, with
// the global test secret.
func signedRequest(t *testing.T, body []byte) *http.Request {
	t.Helper()
	return signedRequestWith(t, testSecret, body)
}

func signedRequestWith(t *testing.T, secret string, body []byte) *http.Request {
	t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderSignature, Sign(secret, ts, body))
	return req
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
	req := signedRequest(t, body)
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
	req := signedRequest(t, body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify meeting was created in store
	meeting, err := s.GetMeeting(ctx, testAccount, "6735648745")
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
	participants, err := s.GetActiveParticipants(ctx, testAccount, "6735648745")
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
	req := signedRequest(t, joinBody)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// Now simulate leave
	leaveBody := loadPayload(t, "participant_left.json")
	req = signedRequest(t, leaveBody)
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
	req := signedRequest(t, body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	meeting, err := s.GetMeeting(ctx, "uUpLO0YDAhWZvYIu_JxPpg", "6704648745")
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
		MeetingID:     "6703648745",
		ZoomAccountID: testAccount,
		Topic:         "Test Meeting",
	}); err != nil {
		t.Fatal(err)
	}

	body := loadPayload(t, "meeting_ended.json")
	req := signedRequest(t, body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	meeting, err := s.GetMeeting(ctx, testAccount, "6703648745")
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
	req := signedRequest(t, []byte(payload))
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
	req := signedRequest(t, []byte(payload))
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
	req := signedRequest(t, body)
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

func TestHandler_RejectsUnsignedEvent(t *testing.T) {
	s, handler := setupHandlerTest(t)
	dispatched := false
	handler.dispatch = func(string, string, WebhookPayload) { dispatched = true }

	body := loadPayload(t, "participant_joined.json")
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if dispatched {
		t.Error("unsigned event must not be dispatched")
	}
	meeting, err := s.GetMeeting(context.Background(), testAccount, "6735648745")
	if err != nil {
		t.Fatal(err)
	}
	if meeting != nil {
		t.Error("unsigned event must not modify the store")
	}
}

func TestHandler_RejectsTamperedBody(t *testing.T) {
	_, handler := setupHandlerTest(t)
	dispatched := false
	handler.dispatch = func(string, string, WebhookPayload) { dispatched = true }

	req := signedRequest(t, loadPayload(t, "participant_joined.json"))
	// Swap in a different body after signing.
	req.Body = io.NopCloser(bytes.NewReader(loadPayload(t, "participant_left.json")))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if dispatched {
		t.Error("tampered event must not be dispatched")
	}
}

// An unsigned CRC request must not be answered: the CRC response is
// HMAC(secret, plainToken), so answering it lets an attacker obtain a valid
// x-zm-signature for any body by sending plainToken "v0:{ts}:{body}".
func TestHandler_RejectsUnsignedCRC(t *testing.T) {
	_, handler := setupHandlerTest(t)

	body := loadPayload(t, "endpoint_url_validation.json")
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "encryptedToken") {
		t.Error("unsigned CRC must not return an encryptedToken")
	}
}

func TestHandler_RejectsOversizedBody(t *testing.T) {
	_, handler := setupHandlerTest(t)

	req := signedRequest(t, bytes.Repeat([]byte("a"), MaxWebhookBodyBytes+1))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rr.Code)
	}
}

// setAccountSecret gives a Zoom account its own webhook secret.
func setAccountSecret(t *testing.T, s *sqlite.SQLiteStore, accountID, secret string) {
	t.Helper()
	if err := s.UpsertZoomAccount(context.Background(), &store.ZoomAccount{AccountID: accountID, WebhookSecret: secret}); err != nil {
		t.Fatal(err)
	}
}

func TestHandler_PerAccountSecret(t *testing.T) {
	s, handler := setupHandlerTest(t)
	setAccountSecret(t, s, testAccount, "account-secret")
	var dispatched int
	handler.dispatch = func(string, string, WebhookPayload) { dispatched++ }
	body := loadPayload(t, "participant_joined.json")

	// Signed with the account's own secret: accepted.
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, signedRequestWith(t, "account-secret", body))
	if rr.Code != http.StatusOK || dispatched != 1 {
		t.Fatalf("account secret: code %d, dispatched %d", rr.Code, dispatched)
	}

	// Signed with the global fallback: rejected, because the account has its
	// own secret. Otherwise whoever holds the global (another Zoom app's)
	// secret could forge this account's events.
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, signedRequestWith(t, testSecret, body))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("fallback secret for account with own secret: expected 401, got %d", rr.Code)
	}
	if dispatched != 1 {
		t.Error("rejected event must not be dispatched")
	}
}

func TestHandler_FallbackSecretForAccountWithoutOwnSecret(t *testing.T) {
	s, handler := setupHandlerTest(t)
	// A different account has its own secret; testAccount does not.
	setAccountSecret(t, s, "OTHER-ACCOUNT", "other-secret")
	body := loadPayload(t, "participant_joined.json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, signedRequestWith(t, testSecret, body))
	if rr.Code != http.StatusOK {
		t.Fatalf("fallback secret: expected 200, got %d", rr.Code)
	}

	// Another account's secret cannot sign for testAccount.
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, signedRequestWith(t, "other-secret", body))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("other account's secret: expected 401, got %d", rr.Code)
	}
}

// CRC requests carry no account ID: any configured secret may sign one, and
// the response must be computed with the secret that verified it.
func TestHandler_CRCWithPerAccountSecret(t *testing.T) {
	s, handler := setupHandlerTest(t)
	setAccountSecret(t, s, "OTHER-ACCOUNT", "other-secret")
	body := loadPayload(t, "endpoint_url_validation.json")

	for _, secret := range []string{testSecret, "other-secret"} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, signedRequestWith(t, secret, body))
		if rr.Code != http.StatusOK {
			t.Fatalf("CRC signed with %q: expected 200, got %d", secret, rr.Code)
		}
		var resp CRCResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		var payload WebhookPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		want, err := ValidateCRC(payload, secret)
		if err != nil {
			t.Fatal(err)
		}
		if resp.EncryptedToken != want.EncryptedToken {
			t.Errorf("CRC signed with %q answered with a different secret", secret)
		}
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, signedRequestWith(t, "unknown-secret", body))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("CRC with unknown secret: expected 401, got %d", rr.Code)
	}
}

// One Zoom account feeding two tenants: state is recorded once, and both
// tenants are notified.
func TestHandler_SharedAccountFansOut(t *testing.T) {
	s, handler := setupHandlerTest(t)
	ctx := context.Background()
	if err := s.CreateTenant(ctx, &store.Tenant{ID: "T2", APIKey: "key-2", ZoomAccountID: testAccount}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var dispatched []string
	handler.dispatch = func(tenantID string, event string, _ WebhookPayload) {
		mu.Lock()
		dispatched = append(dispatched, tenantID)
		mu.Unlock()
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, signedRequest(t, loadPayload(t, "participant_joined.json")))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	mu.Lock()
	sort.Strings(dispatched)
	got := strings.Join(dispatched, ",")
	mu.Unlock()
	if got != "T1,T2" {
		t.Errorf("expected dispatch to T1 and T2, got %s", got)
	}

	participants, err := s.GetActiveParticipants(ctx, testAccount, "6735648745")
	if err != nil {
		t.Fatal(err)
	}
	if len(participants) != 1 {
		t.Errorf("expected one participant row for the shared account, got %d", len(participants))
	}
	for _, tenant := range []string{"T1", "T2"} {
		meetings, err := s.ListActiveMeetings(ctx, tenant)
		if err != nil {
			t.Fatal(err)
		}
		if len(meetings) != 1 {
			t.Errorf("%s: expected the shared meeting, got %d", tenant, len(meetings))
		}
	}
}
