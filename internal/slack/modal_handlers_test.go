package slack

import (
	"context"
	"fmt"
	"testing"
	"time"

	slacklib "github.com/slack-go/slack"
	"github.com/stahnma/zoom-notifier/internal/store"
)

// mockModalOpener records calls to OpenView for testing.
type mockModalOpener struct {
	calls []mockOpenViewCall
}

type mockOpenViewCall struct {
	TriggerID string
	View      slacklib.ModalViewRequest
}

func (m *mockModalOpener) OpenView(triggerID string, view slacklib.ModalViewRequest) (*slacklib.ViewResponse, error) {
	m.calls = append(m.calls, mockOpenViewCall{TriggerID: triggerID, View: view})
	return &slacklib.ViewResponse{}, nil
}

func setupModalCommandHandler(t *testing.T) (*CommandHandler, store.Store, *mockModalOpener) {
	t.Helper()
	s := setupOAuthTestStore(t)

	tenant := &store.Tenant{
		ID:          "T-MODAL",
		TeamName:    "Modal Test",
		APIKey:      "modal-test-key",
		InstalledAt: time.Now(),
	}
	botToken := "xoxb-test-token"
	tenant.BotToken = &botToken
	if err := s.CreateTenant(context.Background(), tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := s.AddAdmin(context.Background(), "T-MODAL", "U-ADMIN"); err != nil {
		t.Fatalf("add admin: %v", err)
	}

	h := NewCommandHandler(s)
	mock := &mockModalOpener{}
	h.SetModalOpener(mock)
	return h, s, mock
}

// --- Tests for commands opening modals ---

func TestSetSuffixOpensModalWhenNoArgs(t *testing.T) {
	h, _, _ := setupModalCommandHandler(t)

	// With trigger_id and no args, the command tries to open a modal.
	// Since our mock doesn't actually call Slack, but the code creates a NewSlackModalOpener
	// with the bot token, we need to verify the fallback behavior.
	// The command creates its own SlackModalOpener, not using h.modals directly.
	// So with a real bot token but no actual Slack API, it will fail and fall back to usage text.
	resp, err := h.Handle(context.Background(), SlashCommand{
		TeamID: "T-MODAL", UserID: "U-ADMIN", Text: "set-suffix",
		TriggerID: "trigger123",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The modal open will fail (no real Slack API), so it falls back to usage text
	if resp.Text == "" {
		t.Error("expected fallback usage text when modal open fails")
	}
}

func TestSetSuffixWithArgsSkipsModal(t *testing.T) {
	h, s, _ := setupModalCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-MODAL", UserID: "U-ADMIN", Text: "set-suffix hello world",
		TriggerID: "trigger123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text == "" {
		t.Error("expected response text")
	}

	tenant, _ := s.GetTenant(ctx, "T-MODAL")
	if tenant.DefaultMsgSuffix != "hello world" {
		t.Errorf("expected suffix 'hello world', got '%s'", tenant.DefaultMsgSuffix)
	}
}

func TestSetSuffixNoModalOpenerFallsBack(t *testing.T) {
	s := setupOAuthTestStore(t)
	tenant := &store.Tenant{
		ID: "T-NOMOD", TeamName: "No Modal", APIKey: "key", InstalledAt: time.Now(),
	}
	if err := s.CreateTenant(context.Background(), tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := s.AddAdmin(context.Background(), "T-NOMOD", "U-ADMIN"); err != nil {
		t.Fatalf("add admin: %v", err)
	}

	h := NewCommandHandler(s) // no modal opener set

	resp, err := h.Handle(context.Background(), SlashCommand{
		TeamID: "T-NOMOD", UserID: "U-ADMIN", Text: "set-suffix",
		TriggerID: "trigger123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text == "" {
		t.Error("expected usage text")
	}
}

func TestSetLinkNoArgsWithTriggerID(t *testing.T) {
	h, _, _ := setupModalCommandHandler(t)

	resp, err := h.Handle(context.Background(), SlashCommand{
		TeamID: "T-MODAL", UserID: "U-ADMIN", Text: "set-link",
		TriggerID: "trigger123",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Falls back to usage since real Slack API unavailable
	if resp.Text == "" {
		t.Error("expected fallback usage text")
	}
}

func TestSetLinkWithArgSkipsModal(t *testing.T) {
	h, s, _ := setupModalCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-MODAL", UserID: "U-ADMIN", Text: "set-link on",
		TriggerID: "trigger123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text == "" {
		t.Error("expected response text")
	}

	tenant, _ := s.GetTenant(ctx, "T-MODAL")
	if !tenant.DefaultIncludeLink {
		t.Error("expected default include link to be true")
	}
}

func TestSubscribeNoArgsWithTriggerID(t *testing.T) {
	h, _, _ := setupModalCommandHandler(t)

	resp, err := h.Handle(context.Background(), SlashCommand{
		TeamID: "T-MODAL", UserID: "U-ADMIN", Text: "subscribe",
		TriggerID: "trigger123",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Falls back to usage since real Slack API unavailable
	if resp.Text == "" {
		t.Error("expected fallback usage text")
	}
}

func TestSubscribeWithArgSkipsModal(t *testing.T) {
	h, s, _ := setupModalCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-MODAL", UserID: "U-ADMIN", Text: "subscribe #general",
		TriggerID: "trigger123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text == "" {
		t.Error("expected subscribed message")
	}

	subs, _ := s.ListSubscriptions(ctx, "T-MODAL")
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}
}

func TestFilterNoArgsWithTriggerID(t *testing.T) {
	h, _, _ := setupModalCommandHandler(t)

	resp, err := h.Handle(context.Background(), SlashCommand{
		TeamID: "T-MODAL", UserID: "U-ADMIN", Text: "filter",
		TriggerID: "trigger123",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Falls back to usage since real Slack API unavailable
	if resp.Text == "" {
		t.Error("expected fallback usage text")
	}
}

func TestFilterWithArgSkipsModal(t *testing.T) {
	h, s, _ := setupModalCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-MODAL", UserID: "U-ADMIN", Text: `filter "Daily Standup"`,
		TriggerID: "trigger123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text == "" {
		t.Error("expected filter added message")
	}

	filters, _ := s.ListFilters(ctx, "T-MODAL")
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
}

// --- Tests for submission handlers ---

func TestHandleSetSuffixSubmission_TenantDefault(t *testing.T) {
	h, s, _ := setupModalCommandHandler(t)
	ctx := context.Background()

	suffix := "the meeting"
	payload := InteractionPayload{Type: "view_submission"}
	payload.Team.ID = "T-MODAL"
	payload.View.CallbackID = "set_suffix"
	payload.View.PrivateMetadata = "T-MODAL"
	payload.View.State = &ViewState{
		Values: map[string]map[string]ViewStateValue{
			"filter_block": {
				"filter_select": {Selected: &SelectedOption{Value: "all"}},
			},
			"suffix_block": {
				"suffix_input": {Value: &suffix},
			},
		},
	}

	err := h.handleSetSuffixSubmission(payload)
	if err != nil {
		t.Fatal(err)
	}

	tenant, _ := s.GetTenant(ctx, "T-MODAL")
	if tenant.DefaultMsgSuffix != "the meeting" {
		t.Errorf("expected suffix 'the meeting', got '%s'", tenant.DefaultMsgSuffix)
	}
}

func TestHandleSetSuffixSubmission_FilterOverride(t *testing.T) {
	h, s, _ := setupModalCommandHandler(t)
	ctx := context.Background()

	// Create a filter
	s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T-MODAL", Pattern: "Standup"})
	filters, _ := s.ListFilters(ctx, "T-MODAL")
	filterID := filters[0].ID

	suffix := "standup suffix"
	payload := InteractionPayload{Type: "view_submission"}
	payload.Team.ID = "T-MODAL"
	payload.View.CallbackID = "set_suffix"
	payload.View.PrivateMetadata = "T-MODAL"
	payload.View.State = &ViewState{
		Values: map[string]map[string]ViewStateValue{
			"filter_block": {
				"filter_select": {Selected: &SelectedOption{Value: fmt.Sprintf("filter_%d", filterID)}},
			},
			"suffix_block": {
				"suffix_input": {Value: &suffix},
			},
		},
	}

	err := h.handleSetSuffixSubmission(payload)
	if err != nil {
		t.Fatal(err)
	}

	filters, _ = s.ListFilters(ctx, "T-MODAL")
	if filters[0].MsgSuffix == nil || *filters[0].MsgSuffix != "standup suffix" {
		t.Errorf("expected filter suffix 'standup suffix', got %v", filters[0].MsgSuffix)
	}
}

func TestHandleSetLinkSubmission_TenantDefault(t *testing.T) {
	h, s, _ := setupModalCommandHandler(t)
	ctx := context.Background()

	payload := InteractionPayload{Type: "view_submission"}
	payload.Team.ID = "T-MODAL"
	payload.View.CallbackID = "set_link"
	payload.View.PrivateMetadata = "T-MODAL"
	payload.View.State = &ViewState{
		Values: map[string]map[string]ViewStateValue{
			"filter_block": {
				"filter_select": {Selected: &SelectedOption{Value: "all"}},
			},
			"link_block": {
				"link_input": {Selected: &SelectedOption{Value: "on"}},
			},
		},
	}

	err := h.handleSetLinkSubmission(payload)
	if err != nil {
		t.Fatal(err)
	}

	tenant, _ := s.GetTenant(ctx, "T-MODAL")
	if !tenant.DefaultIncludeLink {
		t.Error("expected default include link to be true")
	}
}

func TestHandleSetLinkSubmission_FilterOverride(t *testing.T) {
	h, s, _ := setupModalCommandHandler(t)
	ctx := context.Background()

	s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T-MODAL", Pattern: "Retro"})
	filters, _ := s.ListFilters(ctx, "T-MODAL")
	filterID := filters[0].ID

	payload := InteractionPayload{Type: "view_submission"}
	payload.Team.ID = "T-MODAL"
	payload.View.CallbackID = "set_link"
	payload.View.PrivateMetadata = "T-MODAL"
	payload.View.State = &ViewState{
		Values: map[string]map[string]ViewStateValue{
			"filter_block": {
				"filter_select": {Selected: &SelectedOption{Value: fmt.Sprintf("filter_%d", filterID)}},
			},
			"link_block": {
				"link_input": {Selected: &SelectedOption{Value: "off"}},
			},
		},
	}

	err := h.handleSetLinkSubmission(payload)
	if err != nil {
		t.Fatal(err)
	}

	filters, _ = s.ListFilters(ctx, "T-MODAL")
	if filters[0].IncludeLink == nil || *filters[0].IncludeLink {
		t.Error("expected filter include link to be false")
	}
}

func TestHandleSubscribeSubmission(t *testing.T) {
	h, s, _ := setupModalCommandHandler(t)
	ctx := context.Background()

	channelID := "C12345"
	payload := InteractionPayload{Type: "view_submission"}
	payload.Team.ID = "T-MODAL"
	payload.View.CallbackID = "subscribe"
	payload.View.PrivateMetadata = "T-MODAL"
	payload.View.State = &ViewState{
		Values: map[string]map[string]ViewStateValue{
			"channel_block": {
				"channel_select": {SelectedConversation: &channelID},
			},
		},
	}

	err := h.handleSubscribeSubmission(payload)
	if err != nil {
		t.Fatal(err)
	}

	subs, _ := s.ListSubscriptions(ctx, "T-MODAL")
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}
	if subs[0].Target != "C12345" {
		t.Errorf("expected target C12345, got %s", subs[0].Target)
	}
}

func TestHandleAddFilterSubmission(t *testing.T) {
	h, s, _ := setupModalCommandHandler(t)
	ctx := context.Background()

	pattern := "Daily Standup"
	suffix := "the standup"
	payload := InteractionPayload{Type: "view_submission"}
	payload.Team.ID = "T-MODAL"
	payload.View.CallbackID = "add_filter"
	payload.View.PrivateMetadata = "T-MODAL"
	payload.View.State = &ViewState{
		Values: map[string]map[string]ViewStateValue{
			"pattern_block": {
				"pattern_input": {Value: &pattern},
			},
			"suffix_block": {
				"suffix_input": {Value: &suffix},
			},
			"link_block": {
				"link_input": {Selected: &SelectedOption{Value: "on"}},
			},
		},
	}

	err := h.handleAddFilterSubmission(payload)
	if err != nil {
		t.Fatal(err)
	}

	filters, _ := s.ListFilters(ctx, "T-MODAL")
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].Pattern != "Daily Standup" {
		t.Errorf("expected pattern 'Daily Standup', got '%s'", filters[0].Pattern)
	}
	if filters[0].MsgSuffix == nil || *filters[0].MsgSuffix != "the standup" {
		t.Errorf("expected suffix 'the standup', got %v", filters[0].MsgSuffix)
	}
	if filters[0].IncludeLink == nil || !*filters[0].IncludeLink {
		t.Error("expected include link to be true")
	}
}

func TestHandleAddFilterSubmission_DefaultLink(t *testing.T) {
	h, s, _ := setupModalCommandHandler(t)
	ctx := context.Background()

	pattern := "Retro"
	payload := InteractionPayload{Type: "view_submission"}
	payload.Team.ID = "T-MODAL"
	payload.View.CallbackID = "add_filter"
	payload.View.PrivateMetadata = "T-MODAL"
	payload.View.State = &ViewState{
		Values: map[string]map[string]ViewStateValue{
			"pattern_block": {
				"pattern_input": {Value: &pattern},
			},
			"suffix_block": {
				"suffix_input": {},
			},
			"link_block": {
				"link_input": {Selected: &SelectedOption{Value: "default"}},
			},
		},
	}

	err := h.handleAddFilterSubmission(payload)
	if err != nil {
		t.Fatal(err)
	}

	filters, _ := s.ListFilters(ctx, "T-MODAL")
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].IncludeLink != nil {
		t.Error("expected include link to be nil (default)")
	}
	if filters[0].MsgSuffix != nil {
		t.Error("expected suffix to be nil")
	}
}

func TestHandleUnsubscribeSubmission(t *testing.T) {
	h, s, _ := setupModalCommandHandler(t)
	ctx := context.Background()

	// Create a subscription
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T-MODAL", Type: "slack", Target: "C99999", Enabled: true,
	})

	subs, _ := s.ListSubscriptions(ctx, "T-MODAL")
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription before unsubscribe, got %d", len(subs))
	}

	payload := InteractionPayload{Type: "view_submission"}
	payload.Team.ID = "T-MODAL"
	payload.View.CallbackID = "unsubscribe"
	payload.View.PrivateMetadata = "T-MODAL"
	payload.View.State = &ViewState{
		Values: map[string]map[string]ViewStateValue{
			"channel_block": {
				"channel_select": {Selected: &SelectedOption{Value: "C99999"}},
			},
		},
	}

	err := h.handleUnsubscribeSubmission(payload)
	if err != nil {
		t.Fatalf("handleUnsubscribeSubmission: %v", err)
	}

	subs, _ = s.ListSubscriptions(ctx, "T-MODAL")
	if len(subs) != 0 {
		t.Errorf("expected 0 subscriptions after unsubscribe, got %d", len(subs))
	}
}

func TestHandleUnsubscribeSubmission_NotFound(t *testing.T) {
	h, _, _ := setupModalCommandHandler(t)

	payload := InteractionPayload{Type: "view_submission"}
	payload.Team.ID = "T-MODAL"
	payload.View.CallbackID = "unsubscribe"
	payload.View.PrivateMetadata = "T-MODAL"
	payload.View.State = &ViewState{
		Values: map[string]map[string]ViewStateValue{
			"channel_block": {
				"channel_select": {Selected: &SelectedOption{Value: "C-NONEXISTENT"}},
			},
		},
	}

	err := h.handleUnsubscribeSubmission(payload)
	if err == nil {
		t.Error("expected error when subscription not found")
	}
}

func TestRegisterModalHandlers(t *testing.T) {
	h, _, _ := setupModalCommandHandler(t)
	ih := NewInteractionHandler(nil, "secret")

	h.RegisterModalHandlers(ih)

	// Verify all 5 handlers are registered
	expectedCallbacks := []string{"set_suffix", "set_link", "subscribe", "unsubscribe", "add_filter"}
	for _, cb := range expectedCallbacks {
		if _, ok := ih.handlers[cb]; !ok {
			t.Errorf("expected handler for callback_id %q to be registered", cb)
		}
	}
}
