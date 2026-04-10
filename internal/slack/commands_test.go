package slack

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stahnma/zoom-notifier/internal/store"
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

func TestParseChannelName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<#C123|bottery-new>", "bottery-new"},
		{"<#C123>", ""},
		{"#bottery-new", ""},
		{"bottery-new", ""},
		{"", ""},
		{"<#C05MXNTTHM4|general>", "general"},
	}
	for _, tc := range tests {
		got := parseChannelName(tc.input)
		if got != tc.want {
			t.Errorf("parseChannelName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
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
	if !strings.Contains(resp.Text, "status") {
		t.Error("expected help to mention status")
	}
	if strings.Contains(resp.Text, "Admin commands") {
		t.Error("non-admin should not see admin commands")
	}

	// Admin should see admin commands
	resp, err = h.Handle(context.Background(), SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "help",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Admin commands") {
		t.Error("admin should see admin commands")
	}
	if !strings.Contains(resp.Text, "subscribe") {
		t.Error("admin help should mention subscribe")
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
	_ = s.UpsertMeeting(ctx, &store.ActiveMeeting{
		MeetingID: "m1",
		TenantID:  "T-CMD",
		Topic:     "Standup",
		StartTime: time.Now(),
	})
	_ = s.AddParticipant(ctx, &store.Participant{
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

	_ = s.UpsertMeeting(ctx, &store.ActiveMeeting{
		MeetingID: "m2",
		TenantID:  "T-CMD",
		Topic:     "Retro",
		StartTime: time.Now(),
	})
	_ = s.AddParticipant(ctx, &store.Participant{
		MeetingID: "m2", UserName: "Bob", JoinTime: time.Now(),
	})
	_ = s.AddParticipant(ctx, &store.Participant{
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
	_ = s.CreateSubscription(ctx, &store.Subscription{
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

func TestCommandUnsubscribeExactTargetMatch(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	// Subscription stored with plain name "general"
	_ = s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T-CMD", Type: "slack", Target: "general", Enabled: true,
	})

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "unsubscribe general",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Unsubscribed") {
		t.Errorf("expected unsubscribed message, got: %s", resp.Text)
	}

	subs, _ := s.ListSubscriptions(ctx, "T-CMD")
	if len(subs) != 0 {
		t.Errorf("expected 0 subscriptions after unsubscribe, got %d", len(subs))
	}
}

func TestCommandUnsubscribeNoMatch(t *testing.T) {
	h, s := setupCommandHandler(t)
	ctx := context.Background()

	_ = s.CreateSubscription(ctx, &store.Subscription{
		TenantID: "T-CMD", Type: "slack", Target: "general", Enabled: true,
	})

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "unsubscribe #random",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "No Slack subscription found") {
		t.Errorf("expected no subscription found message, got: %s", resp.Text)
	}

	// Subscription should still exist
	subs, _ := s.ListSubscriptions(ctx, "T-CMD")
	if len(subs) != 1 {
		t.Errorf("expected 1 subscription still present, got %d", len(subs))
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
	_ = s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T-CMD", Pattern: "Standup"})

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

func TestCommandAdminsAdd_NoModal(t *testing.T) {
	h, _ := setupCommandHandler(t)

	// Without modal support, admins add shows error
	resp, err := h.Handle(context.Background(), SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "admins add",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Unable to open") {
		t.Errorf("expected unable to open message, got: %s", resp.Text)
	}
}

func TestCommandAdminsList(t *testing.T) {
	h, _ := setupCommandHandler(t)

	resp, err := h.Handle(context.Background(), SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "admins list",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "U-ADMIN") {
		t.Errorf("expected admin list to contain U-ADMIN, got: %s", resp.Text)
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
	_ = s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T-CMD", Pattern: "Standup"})

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
	_ = s.CreateFilter(ctx, &store.MeetingFilter{TenantID: "T-CMD", Pattern: "Retro"})

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
	_ = s.UpdateTenantDefaults(ctx, "T-CMD", "the meeting", true)

	// Create a filter with overrides
	suffix := "standup suffix"
	includeLink := false
	_ = s.CreateFilter(ctx, &store.MeetingFilter{
		TenantID:    "T-CMD",
		Pattern:     "Standup",
		MsgSuffix:   &suffix,
		IncludeLink: &includeLink,
	})

	// Create a filter without overrides
	_ = s.CreateFilter(ctx, &store.MeetingFilter{
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

func TestCommandSetup(t *testing.T) {
	h, _ := setupCommandHandler(t)
	h.SetServerURL("https://example.com")
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "setup",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Open Zoom Setup") {
		t.Errorf("expected setup link in response, got: %s", resp.Text)
	}
	if !strings.Contains(resp.Text, "https://example.com/tenant/setup?key=test-api-key") {
		t.Errorf("expected full setup URL, got: %s", resp.Text)
	}
	if !strings.Contains(resp.Text, "Keep this link private") {
		t.Errorf("expected privacy warning, got: %s", resp.Text)
	}
}

func TestCommandSetupNoServerURL(t *testing.T) {
	h, _ := setupCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-ADMIN", Text: "setup",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "/tenant/setup?key=test-api-key") {
		t.Errorf("expected setup URL with path fallback, got: %s", resp.Text)
	}
}

func TestCommandSetupNonAdmin(t *testing.T) {
	h, _ := setupCommandHandler(t)
	ctx := context.Background()

	resp, err := h.Handle(ctx, SlashCommand{
		TeamID: "T-CMD", UserID: "U-NOBODY", Text: "setup",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "Permission denied") {
		t.Errorf("expected permission denied, got: %s", resp.Text)
	}
}
