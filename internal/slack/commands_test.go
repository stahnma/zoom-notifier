package slack

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

func setupCommandHandler(t *testing.T) (*CommandHandler, store.Store) {
	t.Helper()
	s := setupOAuthTestStore(t) // reuse from oauth_test.go

	// Create a test tenant
	tenant := &store.Tenant{
		ID:          "T-CMD",
		TeamName:    "Command Test",
		APIKey:      "test-api-key",
		InstalledAt: time.Now(),
	}
	if err := s.CreateTenant(context.Background(), tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	// Add an admin
	if err := s.AddAdmin(context.Background(), "T-CMD", "U-ADMIN"); err != nil {
		t.Fatalf("add admin: %v", err)
	}

	return NewCommandHandler(s), s
}

func TestCommandHelp(t *testing.T) {
	h, _ := setupCommandHandler(t)

	// Empty text → help
	resp, err := h.Handle(context.Background(), SlashCommand{
		TeamID: "T-CMD", UserID: "U-ANYONE", Text: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ResponseType != "ephemeral" {
		t.Error("expected ephemeral response")
	}
	if !strings.Contains(resp.Text, "zoom-notifier commands") {
		t.Error("expected help text")
	}

	// Explicit help
	resp, err = h.Handle(context.Background(), SlashCommand{
		TeamID: "T-CMD", UserID: "U-ANYONE", Text: "help",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "subscribe") {
		t.Error("expected help to mention subscribe")
	}
}

func TestCommandUnknown(t *testing.T) {
	h, _ := setupCommandHandler(t)

	resp, err := h.Handle(context.Background(), SlashCommand{
		TeamID: "T-CMD", UserID: "U-ANYONE", Text: "foobar",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Unknown command") {
		t.Errorf("expected unknown command message, got: %s", resp.Text)
	}
}

func TestCommandStatusNoMeetings(t *testing.T) {
	h, _ := setupCommandHandler(t)

	resp, err := h.Handle(context.Background(), SlashCommand{
		TeamID: "T-CMD", UserID: "U-ANYONE", Text: "status",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "No active meetings") {
		t.Errorf("expected no meetings message, got: %s", resp.Text)
	}
}

func TestCommandStatusWithMeetings(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	// Create a meeting with a participant
	s.UpsertMeeting(ctx, &store.ActiveMeeting{
		MeetingID: "m1",
		TenantID:  "T-CMD",
		Topic:     "Standup",
		StartTime: time.Now(),
	})
	s.AddParticipant(ctx, &store.Participant{
		MeetingID: "m1",
		UserName:  "Alice",
		JoinTime:  time.Now(),
	})

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ANYONE", Text: "status",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Standup") {
		t.Errorf("expected meeting topic in status, got: %s", resp.Text)
	}
	if !strings.Contains(resp.Text, "1 participant") {
		t.Errorf("expected participant count, got: %s", resp.Text)
	}
}

func TestCommandWhois(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	s.UpsertMeeting(ctx, &store.ActiveMeeting{
		MeetingID: "m2",
		TenantID:  "T-CMD",
		Topic:     "Retro",
		StartTime: time.Now(),
	})
	s.AddParticipant(ctx, &store.Participant{
		MeetingID: "m2", UserName: "Bob", JoinTime: time.Now(),
	})
	s.AddParticipant(ctx, &store.Participant{
		MeetingID: "m2", UserName: "Carol", JoinTime: time.Now(),
	})

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ANYONE", Text: "whois Retro",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Bob") || !strings.Contains(resp.Text, "Carol") {
		t.Errorf("expected participants in whois, got: %s", resp.Text)
	}
}

func TestCommandWhoisNotFound(t *testing.T) {
	h, _ := setupCommandHandler(t)

	resp, err := h.Handle(context.Background(), SlashCommand{
		TeamID: "T-CMD", UserID: "U-ANYONE", Text: "whois NonExistent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "No active meeting found") {
		t.Errorf("expected not found message, got: %s", resp.Text)
	}
}

func TestCommandSubscribeAsAdmin(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "subscribe #general",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Subscribed") {
		t.Errorf("expected subscribed message, got: %s", resp.Text)
	}

	// Verify subscription was created
	subs, err := s.ListSubscriptions(ctx, "T-CMD")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}
	if subs[0].Target != "general" {
		t.Errorf("expected target general, got %s", subs[0].Target)
	}
}

func TestCommandSubscribeAsNonAdmin(t *testing.T) {
	h, _ := setupCommandHandler(t)

	resp, err := h.Handle(context.Background(), SlashCommand{
		TeamID: "T-CMD", UserID: "U-NOBODY", Text: "subscribe #general",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Permission denied") {
		t.Errorf("expected permission denied, got: %s", resp.Text)
	}
}

func TestCommandUnsubscribe(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	// Create a subscription first
	s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T-CMD", Type: "slack", Target: "alerts", Enabled: true,
	})

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "unsubscribe #alerts",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Unsubscribed") {
		t.Errorf("expected unsubscribed message, got: %s", resp.Text)
	}
}

func TestCommandFilter(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "filter \"Daily Standup\"",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Added meeting filter") {
		t.Errorf("expected filter added message, got: %s", resp.Text)
	}

	filters, err := s.ListFilters(ctx, "T-CMD")
	if err != nil {
		t.Fatal(err)
	}
	if len(filters) != 1 || filters[0].Pattern != "Daily Standup" {
		t.Errorf("expected filter 'Daily Standup', got %v", filters)
	}
}

func TestCommandFilters(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	// No filters
	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ANYONE", Text: "filters",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "No meeting filters") {
		t.Errorf("expected no filters message, got: %s", resp.Text)
	}

	// Add a filter
	s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T-CMD", Pattern: "Standup"})

	resp, err = h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ANYONE", Text: "filters",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Standup") {
		t.Errorf("expected filter in list, got: %s", resp.Text)
	}
}

func TestCommandAdminsAdd(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "admins add <@U-NEW|newuser>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Added") {
		t.Errorf("expected added message, got: %s", resp.Text)
	}

	isAdmin, err := s.IsAdmin(ctx, "T-CMD", "U-NEW")
	if err != nil {
		t.Fatal(err)
	}
	if !isAdmin {
		t.Error("expected U-NEW to be admin")
	}
}

func TestCommandAPIKey(t *testing.T) {
	h, _ := setupCommandHandler(t)

	resp, err := h.Handle(context.Background(), SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "api-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "test-api-key") {
		t.Errorf("expected API key in response, got: %s", resp.Text)
	}
	if resp.ResponseType != "ephemeral" {
		t.Error("api-key response should be ephemeral")
	}
}

func TestCommandSetSuffixTenantDefault(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: `set-suffix "the daily standup"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Updated default message suffix") {
		t.Errorf("expected updated message, got: %s", resp.Text)
	}

	tenant, _ := s.GetTenant(ctx, "T-CMD")
	if tenant.DefaultMsgSuffix != "the daily standup" {
		t.Errorf("expected tenant default suffix 'the daily standup', got '%s'", tenant.DefaultMsgSuffix)
	}
}

func TestCommandSetSuffixFilterOverride(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	// Create a filter first
	s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T-CMD", Pattern: "Standup"})

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: `set-suffix "Standup" "the standup meeting"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Updated suffix override on filter") {
		t.Errorf("expected filter override message, got: %s", resp.Text)
	}

	// Verify filter was updated
	filters, _ := s.ListFilters(ctx, "T-CMD")
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].MsgSuffix == nil || *filters[0].MsgSuffix != "the standup meeting" {
		t.Errorf("expected filter suffix 'the standup meeting', got %v", filters[0].MsgSuffix)
	}
}

func TestCommandSetSuffixFilterNotFound(t *testing.T) {
	h, _ := setupCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: `set-suffix "NonExistent" "some text"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "No filter found") {
		t.Errorf("expected no filter found message, got: %s", resp.Text)
	}
}

func TestCommandSetSuffixNoArgs(t *testing.T) {
	h, _ := setupCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "set-suffix",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Usage") {
		t.Errorf("expected usage message, got: %s", resp.Text)
	}
}

func TestCommandSetLinkTenantDefault(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "set-link on",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "enabled") {
		t.Errorf("expected enabled message, got: %s", resp.Text)
	}

	tenant, _ := s.GetTenant(ctx, "T-CMD")
	if !tenant.DefaultIncludeLink {
		t.Error("expected tenant default_include_link to be true")
	}

	// Test off
	resp, err = h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "set-link off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "disabled") {
		t.Errorf("expected disabled message, got: %s", resp.Text)
	}

	tenant, _ = s.GetTenant(ctx, "T-CMD")
	if tenant.DefaultIncludeLink {
		t.Error("expected tenant default_include_link to be false")
	}
}

func TestCommandSetLinkFilterOverride(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	// Create a filter
	s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T-CMD", Pattern: "Retro"})

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: `set-link "Retro" on`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "enabled for filter") {
		t.Errorf("expected filter override message, got: %s", resp.Text)
	}

	// Verify filter was updated
	filters, _ := s.ListFilters(ctx, "T-CMD")
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].IncludeLink == nil || !*filters[0].IncludeLink {
		t.Error("expected filter include_link to be true")
	}
}

func TestCommandSetLinkFilterNotFound(t *testing.T) {
	h, _ := setupCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: `set-link "NonExistent" on`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "No filter found") {
		t.Errorf("expected no filter found message, got: %s", resp.Text)
	}
}

func TestCommandSetLinkNoArgs(t *testing.T) {
	h, _ := setupCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "set-link",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Usage") {
		t.Errorf("expected usage message, got: %s", resp.Text)
	}
}

func TestCommandSettings(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	// Set some defaults
	s.UpdateTenantDefaults(ctx, "T-CMD", "the meeting", true)

	// Create a filter with overrides
	suffix := "standup suffix"
	includeLink := false
	s.CreateFilter(ctx, &store.MeetingFilter{
		TenantID:    "T-CMD",
		Pattern:     "Standup",
		MsgSuffix:   &suffix,
		IncludeLink: &includeLink,
	})

	// Create a filter without overrides
	s.CreateFilter(ctx, &store.MeetingFilter{
		TenantID: "T-CMD",
		Pattern:  "Retro",
	})

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ANYONE", Text: "settings",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(resp.Text, "Notification Settings") {
		t.Errorf("expected settings header, got: %s", resp.Text)
	}
	if !strings.Contains(resp.Text, "the meeting") {
		t.Errorf("expected default suffix in settings, got: %s", resp.Text)
	}
	if !strings.Contains(resp.Text, "on") {
		t.Errorf("expected include link on, got: %s", resp.Text)
	}
	if !strings.Contains(resp.Text, "Standup") {
		t.Errorf("expected Standup filter, got: %s", resp.Text)
	}
	if !strings.Contains(resp.Text, "standup suffix") {
		t.Errorf("expected filter suffix override, got: %s", resp.Text)
	}
	if !strings.Contains(resp.Text, "no overrides") {
		t.Errorf("expected 'no overrides' for Retro filter, got: %s", resp.Text)
	}
}

func TestCommandSettingsEmpty(t *testing.T) {
	h, _ := setupCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ANYONE", Text: "settings",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(resp.Text, "(none)") {
		t.Errorf("expected (none) for empty suffix, got: %s", resp.Text)
	}
}
