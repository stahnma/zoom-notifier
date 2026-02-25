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
		ID:            "T1",
		APIKey:        "key-1",
		BotToken:      &botToken,
		ZoomAccountID: "zoom-1",
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
		TenantID: "T1", Type: "slack", Target: "#general",
		MsgSuffix: "the standup.", IncludeLink: false, Enabled: true,
	})
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#dev",
		MsgSuffix: "the standup.", IncludeLink: false, Enabled: true,
	})
	s.UpsertIRCConfig(ctx, &store.IRCConfig{
		TenantID: "T1", Server: "irc.libera.chat:6697", Nick: "bot", Password: "pass", UseTLS: true,
	})
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "irc", Target: "#irc-channel",
		MsgSuffix: "the standup.", IncludeLink: false, Enabled: true,
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
		TenantID: "T1", Type: "slack", Target: "#general",
		MsgSuffix: "the meeting.", Enabled: true,
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
		TenantID: "T1", Type: "slack", Target: "#general",
		MsgSuffix: "the standup.", Enabled: true,
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
		TenantID: "T1", Type: "slack", Target: "#disabled",
		MsgSuffix: "the meeting.", Enabled: false,
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
		TenantID: "T1", Type: "slack", Target: "#general",
		MsgSuffix: "the standup.", Enabled: true,
	})

	payload := makeLeavePayload("Bob", "Daily Standup", "m-500")
	dispatcher.Dispatch(ctx, "T1", payload)

	slackSender.mu.Lock()
	if len(slackSender.calls) != 1 {
		t.Fatalf("expected 1 slack call, got %d", len(slackSender.calls))
	}
	msg := slackSender.calls[0].msg
	if msg != "Bob has left the standup." {
		t.Errorf("unexpected message: '%s'", msg)
	}
	slackSender.mu.Unlock()
}

func TestDispatchJoinMessage(t *testing.T) {
	s, slackSender, _, dispatcher := setupDispatcherTest(t)
	ctx := context.Background()

	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T1", Type: "slack", Target: "#general",
		MsgSuffix: "the zoom meeting.", Enabled: true,
	})

	payload := makeJoinPayload("Alice", "My Meeting", "m-600")
	dispatcher.Dispatch(ctx, "T1", payload)

	slackSender.mu.Lock()
	if len(slackSender.calls) != 1 {
		t.Fatalf("expected 1 slack call, got %d", len(slackSender.calls))
	}
	msg := slackSender.calls[0].msg
	if msg != "Alice has joined the zoom meeting." {
		t.Errorf("unexpected message: '%s'", msg)
	}
	slackSender.mu.Unlock()
}
