package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stahnma/zoom-notifier/internal/store"
	"github.com/stahnma/zoom-notifier/internal/store/sqlite"
	"github.com/stahnma/zoom-notifier/internal/zoom"
)

type mockSlackSender struct {
	mu    sync.Mutex
	calls []slackCall
}

type slackCall struct {
	botToken  string
	channelID string
	msg       string
}

func (m *mockSlackSender) Send(ctx context.Context, botToken string, channelID string, msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, slackCall{botToken: botToken, channelID: channelID, msg: msg})
	return nil
}

type mockIRCSender struct {
	mu    sync.Mutex
	calls []ircCall
}

type ircCall struct {
	channel string
	msg     string
}

func (m *mockIRCSender) Send(ctx context.Context, config *store.IRCConfig, channel string, msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, ircCall{channel: channel, msg: msg})
	return nil
}

func setupDispatcherTest(t *testing.T) (*sqlite.SQLiteStore, *mockSlackSender, *mockIRCSender, *Dispatcher) {
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

	ctx := context.Background()
	botToken := "xoxb-test-token"
	if err := s.CreateTenant(ctx, &store.Tenant{
		ID:               "T1",
		APIKey:           "key-1",
		BotToken:         &botToken,
		ZoomAccountID:    "zoom-1",
		DefaultMsgSuffix: "the meeting.",
	}); err != nil {
		t.Fatal(err)
	}

	slackSender := &mockSlackSender{}
	ircSender := &mockIRCSender{}
	dispatcher := NewDispatcher(s, slackSender, ircSender)

	return s, slackSender, ircSender, dispatcher
}

func makeJoinPayload(userName, topic, meetingID string) zoom.WebhookPayload {
	var p zoom.WebhookPayload
	p.Event = "meeting.participant_joined"
	p.Payload.Object.Participant.UserName = userName
	p.Payload.Object.Topic = topic
	p.Payload.Object.ID = meetingID
	return p
}

func makeLeavePayload(userName, topic, meetingID string) zoom.WebhookPayload {
	var p zoom.WebhookPayload
	p.Event = "meeting.participant_left"
	p.Payload.Object.Participant.UserName = userName
	p.Payload.Object.Topic = topic
	p.Payload.Object.ID = meetingID
	return p
}

func TestDispatchFanOut(t *testing.T) {
	s, slackSender, ircSender, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	// Create 2 slack subs and 1 IRC sub
	if err := s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#dev", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertIRCConfig(ctx, &store.IRCConfig{
		TenantID: "T1", Server: "irc.libera.chat:6697", Nick: "bot", Password: "pass", UseTLS: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "irc", Target: "#irc-channel", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	payload := makeJoinPayload("Alice", "Daily Standup", "m-100")
	dispatcher.Dispatch(ctx, "T1", payload)

	slackSender.mu.Lock()
	if len(slackSender.calls) != 2 {
		t.Errorf("expected 2 slack calls, got %d", len(slackSender.calls))
	}
	slackSender.mu.Unlock()

	ircSender.mu.Lock()
	if len(ircSender.calls) != 1 {
		t.Errorf("expected 1 IRC call, got %d", len(ircSender.calls))
	}
	ircSender.mu.Unlock()
}

func TestDispatchRespectsFilters(t *testing.T) {
	s, slackSender, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	if err := s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T1", Pattern: "Daily Standup"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	// "All Hands" does not match filter "Daily Standup"
	payload := makeJoinPayload("Alice", "All Hands", "m-200")
	dispatcher.Dispatch(ctx, "T1", payload)

	slackSender.mu.Lock()
	if len(slackSender.calls) != 0 {
		t.Errorf("expected 0 slack calls (filtered out), got %d", len(slackSender.calls))
	}
	slackSender.mu.Unlock()
}

func TestDispatchFilters_MatchingTopic(t *testing.T) {
	s, slackSender, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	if err := s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T1", Pattern: "Daily Standup"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	// "Daily Standup" matches the filter
	payload := makeJoinPayload("Alice", "Daily Standup", "m-300")
	dispatcher.Dispatch(ctx, "T1", payload)

	slackSender.mu.Lock()
	if len(slackSender.calls) != 1 {
		t.Errorf("expected 1 slack call (matching filter), got %d", len(slackSender.calls))
	}
	slackSender.mu.Unlock()
}

func TestDispatchDisabledSubscription(t *testing.T) {
	s, slackSender, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	if err := s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#disabled", Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}

	payload := makeJoinPayload("Alice", "Any Meeting", "m-400")
	dispatcher.Dispatch(ctx, "T1", payload)

	slackSender.mu.Lock()
	if len(slackSender.calls) != 0 {
		t.Errorf("expected 0 slack calls (disabled sub), got %d", len(slackSender.calls))
	}
	slackSender.mu.Unlock()
}

func TestDispatchLeaveMessage(t *testing.T) {
	s, slackSender, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	if err := s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	payload := makeLeavePayload("Bob", "Daily Standup", "m-500")
	dispatcher.Dispatch(ctx, "T1", payload)

	slackSender.mu.Lock()
	if len(slackSender.calls) != 1 {
		t.Fatalf("expected 1 slack call, got %d", len(slackSender.calls))
	}
	msg := slackSender.calls[0].msg
	// Tenant default suffix is "the meeting."
	if msg != "Bob has left the meeting." {
		t.Errorf("unexpected message: '%s'", msg)
	}
	slackSender.mu.Unlock()
}

func TestDispatchJoinMessage(t *testing.T) {
	s, slackSender, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	if err := s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	payload := makeJoinPayload("Alice", "My Meeting", "m-600")
	dispatcher.Dispatch(ctx, "T1", payload)

	slackSender.mu.Lock()
	if len(slackSender.calls) != 1 {
		t.Fatalf("expected 1 slack call, got %d", len(slackSender.calls))
	}
	msg := slackSender.calls[0].msg
	// Tenant default suffix is "the meeting."
	if msg != "Alice has joined the meeting." {
		t.Errorf("unexpected message: '%s'", msg)
	}
	slackSender.mu.Unlock()
}

func TestDispatchUsesTenantDefaultSuffix(t *testing.T) {
	s, slackSender, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	// Update tenant default suffix
	if err := s.UpdateTenantDefaults(ctx, "T1", "the zoom call.", false); err != nil {
		t.Fatal(err)
	}

	if err := s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	payload := makeJoinPayload("Alice", "My Meeting", "m-700")
	dispatcher.Dispatch(ctx, "T1", payload)

	slackSender.mu.Lock()
	if len(slackSender.calls) != 1 {
		t.Fatalf("expected 1 slack call, got %d", len(slackSender.calls))
	}
	msg := slackSender.calls[0].msg
	if msg != "Alice has joined the zoom call." {
		t.Errorf("expected 'Alice has joined the zoom call.', got '%s'", msg)
	}
	slackSender.mu.Unlock()
}

func TestDispatchFilterOverrideTakesPrecedence(t *testing.T) {
	s, slackSender, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	// Tenant default is "the meeting." (set in setupDispatcherTest)
	// Filter override for "Daily Standup" uses "the standup."
	filterSuffix := "the standup."
	if err := s.CreateFilter(ctx, &store.MeetingFilter{
		TenantID:  "T1",
		Pattern:   "Daily Standup",
		MsgSuffix: &filterSuffix,
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	payload := makeJoinPayload("Alice", "Daily Standup", "m-800")
	dispatcher.Dispatch(ctx, "T1", payload)

	slackSender.mu.Lock()
	if len(slackSender.calls) != 1 {
		t.Fatalf("expected 1 slack call, got %d", len(slackSender.calls))
	}
	msg := slackSender.calls[0].msg
	if msg != "Alice has joined the standup." {
		t.Errorf("expected 'Alice has joined the standup.', got '%s'", msg)
	}
	slackSender.mu.Unlock()
}

func TestDispatchFilterWithoutOverrideUsesTenantDefault(t *testing.T) {
	s, slackSender, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	// Filter without suffix override -- should fall back to tenant default
	if err := s.CreateFilter(ctx, &store.MeetingFilter{
		TenantID: "T1",
		Pattern:  "Daily Standup",
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	payload := makeJoinPayload("Alice", "Daily Standup", "m-900")
	dispatcher.Dispatch(ctx, "T1", payload)

	slackSender.mu.Lock()
	if len(slackSender.calls) != 1 {
		t.Fatalf("expected 1 slack call, got %d", len(slackSender.calls))
	}
	msg := slackSender.calls[0].msg
	// Tenant default is "the meeting."
	if msg != "Alice has joined the meeting." {
		t.Errorf("expected 'Alice has joined the meeting.', got '%s'", msg)
	}
	slackSender.mu.Unlock()
}

func TestDispatchSuffixAppliedToAllSubscriptions(t *testing.T) {
	s, slackSender, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	// Create two subscriptions -- both should get the same suffix
	if err := s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#dev", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	payload := makeJoinPayload("Alice", "My Meeting", "m-1000")
	dispatcher.Dispatch(ctx, "T1", payload)

	slackSender.mu.Lock()
	if len(slackSender.calls) != 2 {
		t.Fatalf("expected 2 slack calls, got %d", len(slackSender.calls))
	}
	for _, call := range slackSender.calls {
		if call.msg != "Alice has joined the meeting." {
			t.Errorf("expected 'Alice has joined the meeting.', got '%s'", call.msg)
		}
	}
	slackSender.mu.Unlock()
}

func TestGetMeetingLink_NoCredentials(t *testing.T) {
	s, _, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	// Account exists (linked by T1) but has no API credentials.
	if a, err := s.GetZoomAccount(ctx, "zoom-1"); err != nil || a == nil {
		t.Fatalf("expected zoom-1 account row: %+v, %v", a, err)
	}
	if link := dispatcher.getMeetingLink(ctx, "zoom-1", "m-999"); link != "" {
		t.Errorf("expected empty string when no zoom credentials, got '%s'", link)
	}
}

func TestGetMeetingLink_UnknownOrEmptyAccount(t *testing.T) {
	_, _, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	for _, acct := range []string{"", "ACCT-DOESNOTEXIST"} {
		if link := dispatcher.getMeetingLink(ctx, acct, "m-999"); link != "" {
			t.Errorf("account %q: expected empty string, got '%s'", acct, link)
		}
	}
}

// fakeZoom is a minimal Zoom OAuth + meetings API that records which client ID
// each token was issued to and returns a join URL naming that client.
type fakeZoom struct {
	mu          sync.Mutex
	tokenGrants []string // client IDs, in order of token requests
}

func newFakeZoom(t *testing.T) (*fakeZoom, *httptest.Server) {
	t.Helper()
	fz := &fakeZoom{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		clientID, _, ok := r.BasicAuth()
		if !ok {
			http.Error(w, "no basic auth", http.StatusUnauthorized)
			return
		}
		fz.mu.Lock()
		fz.tokenGrants = append(fz.tokenGrants, clientID)
		fz.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token-" + clientID,
			"token_type":   "bearer",
			"expires_in":   3600,
		}); err != nil {
			t.Errorf("encode token response: %v", err)
		}
	})
	mux.HandleFunc("/v2/meetings/", func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer token-")
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"join_url": "https://zoom.example/j/" + token,
		}); err != nil {
			t.Errorf("encode meeting response: %v", err)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return fz, srv
}

func (fz *fakeZoom) grants() []string {
	fz.mu.Lock()
	defer fz.mu.Unlock()
	return append([]string(nil), fz.tokenGrants...)
}

func useFakeZoom(t *testing.T, d *Dispatcher) *fakeZoom {
	t.Helper()
	fz, srv := newFakeZoom(t)
	d.zoomOAuthURL = srv.URL
	d.zoomAPIURL = srv.URL
	return fz
}

func setAPICreds(t *testing.T, s *sqlite.SQLiteStore, accountID, clientID string) {
	t.Helper()
	if err := s.UpsertZoomAccount(context.Background(), &store.ZoomAccount{
		AccountID: accountID, ClientID: clientID, ClientSecret: "secret-" + clientID,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestGetMeetingLink_RebuildsClientWhenCredentialsChange(t *testing.T) {
	s, _, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()
	fz := useFakeZoom(t, dispatcher)

	setAPICreds(t, s, "zoom-1", "old")
	if got := dispatcher.getMeetingLink(ctx, "zoom-1", "m1"); got != "https://zoom.example/j/old" {
		t.Fatalf("first link = %q", got)
	}
	// Unchanged credentials reuse the cached client and its token.
	if got := dispatcher.getMeetingLink(ctx, "zoom-1", "m1"); got != "https://zoom.example/j/old" {
		t.Fatalf("second link = %q", got)
	}
	if g := fz.grants(); len(g) != 1 {
		t.Fatalf("expected 1 token request with unchanged creds, got %v", g)
	}

	setAPICreds(t, s, "zoom-1", "new")
	if got := dispatcher.getMeetingLink(ctx, "zoom-1", "m1"); got != "https://zoom.example/j/new" {
		t.Fatalf("link after rotation = %q, want one fetched with new credentials", got)
	}
	if g := fz.grants(); len(g) != 2 || g[1] != "new" {
		t.Fatalf("expected a token request with the new client ID, got %v", g)
	}
}

func TestGetMeetingLink_ForgetsClientWhenCredentialsRemoved(t *testing.T) {
	s, _, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()
	useFakeZoom(t, dispatcher)

	setAPICreds(t, s, "zoom-1", "c")
	if got := dispatcher.getMeetingLink(ctx, "zoom-1", "m1"); got == "" {
		t.Fatal("expected a link while credentials exist")
	}

	if err := s.UpsertZoomAccount(ctx, &store.ZoomAccount{AccountID: "zoom-1"}); err != nil { // clear credentials
		t.Fatal(err)
	}
	if got := dispatcher.getMeetingLink(ctx, "zoom-1", "m1"); got != "" {
		t.Fatalf("expected no link after credentials were removed, got %q", got)
	}
	dispatcher.mu.Lock()
	_, cached := dispatcher.zoomClients["zoom-1"]
	dispatcher.mu.Unlock()
	if cached {
		t.Error("expected cached client to be dropped after credentials were removed")
	}
}

// Tenants sharing a Zoom account share one API client (and OAuth token).
func TestDispatch_SharedAccountSharesZoomClient(t *testing.T) {
	s, slackSender, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()
	fz := useFakeZoom(t, dispatcher)
	setAPICreds(t, s, "zoom-1", "shared")

	botToken := "xoxb-t2"
	if err := s.CreateTenant(ctx, &store.Tenant{
		ID: "T2", APIKey: "key-2", BotToken: &botToken, ZoomAccountID: "zoom-1",
		DefaultMsgSuffix: "the meeting.", DefaultIncludeLink: true,
	}); err != nil {
		t.Fatal(err)
	}
	t1, err := s.GetTenant(ctx, "T1")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateTenantDefaults(ctx, t1.ID, t1.DefaultMsgSuffix, true); err != nil {
		t.Fatal(err)
	}
	for _, tenant := range []string{"T1", "T2"} {
		if err := s.CreateSubscription(ctx, &store.Subscription{TenantID: tenant, Type: store.SubscriptionTypeSlack, Target: "C-" + tenant, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}

	payload := makeJoinPayload("Alice", "Standup", "m-1")
	dispatcher.Dispatch(ctx, "T1", payload)
	dispatcher.Dispatch(ctx, "T2", payload)

	slackSender.mu.Lock()
	defer slackSender.mu.Unlock()
	if len(slackSender.calls) != 2 {
		t.Fatalf("expected 2 slack sends, got %d", len(slackSender.calls))
	}
	for _, c := range slackSender.calls {
		if !strings.Contains(c.msg, "https://zoom.example/j/shared") {
			t.Errorf("expected join link in %q", c.msg)
		}
	}
	if g := fz.grants(); len(g) != 1 {
		t.Errorf("expected one OAuth token for the shared account, got %v", g)
	}
}
