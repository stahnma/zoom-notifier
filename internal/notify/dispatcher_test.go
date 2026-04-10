package notify

import (
	"context"
	"sync"
	"testing"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store/sqlite"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/zoom"
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
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	botToken := "xoxb-test-token"
	s.CreateTenant(ctx, &store.Tenant{
		ID:               "T1",
		APIKey:           "key-1",
		BotToken:         &botToken,
		ZoomAccountID:    "zoom-1",
		DefaultMsgSuffix: "the meeting.",
	})

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
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	})
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#dev", Enabled: true,
	})
	s.UpsertIRCConfig(ctx, &store.IRCConfig{
		TenantID: "T1", Server: "irc.libera.chat:6697", Nick: "bot", Password: "pass", UseTLS: true,
	})
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "irc", Target: "#irc-channel", Enabled: true,
	})

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

	s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T1", Pattern: "Daily Standup"})
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	})

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

	s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T1", Pattern: "Daily Standup"})
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	})

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

	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#disabled", Enabled: false,
	})

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

	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	})

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

	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	})

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
	s.UpdateTenantDefaults(ctx, "T1", "the zoom call.", false)

	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	})

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
	s.CreateFilter(ctx, &store.MeetingFilter{
		TenantID:  "T1",
		Pattern:   "Daily Standup",
		MsgSuffix: &filterSuffix,
	})

	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	})

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
	s.CreateFilter(ctx, &store.MeetingFilter{
		TenantID: "T1",
		Pattern:  "Daily Standup",
	})

	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	})

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
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general", Enabled: true,
	})
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#dev", Enabled: true,
	})

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

	// Create a tenant with no zoom credentials stored
	s.CreateTenant(ctx, &store.Tenant{
		ID:     "T-NOCREDS",
		APIKey: "key-nocreds",
	})

	link := dispatcher.getMeetingLink(ctx, "T-NOCREDS", "m-999")
	if link != "" {
		t.Errorf("expected empty string when no zoom credentials, got '%s'", link)
	}
}

func TestGetMeetingLink_NonexistentTenant(t *testing.T) {
	_, _, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	link := dispatcher.getMeetingLink(ctx, "T-DOESNOTEXIST", "m-999")
	if link != "" {
		t.Errorf("expected empty string for nonexistent tenant, got '%s'", link)
	}
}
