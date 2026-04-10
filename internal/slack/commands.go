package slack

import (
	"context"
	"fmt"
	"strings"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

// SlashCommand represents a parsed Slack slash command.
type SlashCommand struct {
	TeamID    string
	UserID    string
	ChannelID string
	Text      string
}

// SlashResponse is the response returned to the user.
type SlashResponse struct {
	Text         string
	ResponseType string // "ephemeral" or "in_channel"
}

func ephemeral(text string) *SlashResponse {
	return &SlashResponse{Text: text, ResponseType: "ephemeral"}
}

func inChannel(text string) *SlashResponse {
	return &SlashResponse{Text: text, ResponseType: "in_channel"}
}

// parseChannelID extracts a channel ID from Slack's mention format.
// Slack sends "#general" as "<#C05MXNTTHM4>" or "<#C05MXNTTHM4|general>".
func parseChannelID(s string) string {
	s = strings.TrimPrefix(s, "<#")
	s = strings.TrimSuffix(s, ">")
	if idx := strings.Index(s, "|"); idx >= 0 {
		s = s[:idx]
	}
	return s
}

// CommandHandler routes slash commands to their implementations.
type CommandHandler struct {
	store store.Store
}

func NewCommandHandler(s store.Store) *CommandHandler {
	return &CommandHandler{store: s}
}

// Handle routes a slash command to the appropriate handler.
func (h *CommandHandler) Handle(ctx context.Context, cmd SlashCommand) (*SlashResponse, error) {
	parts := strings.Fields(cmd.Text)
	if len(parts) == 0 {
		return h.help(ctx, cmd)
	}

	switch parts[0] {
	case "status":
		return h.status(ctx, cmd)
	case "whois":
		return h.whois(ctx, cmd, parts[1:])
	case "subscribe":
		return h.requireAdmin(ctx, cmd, func() (*SlashResponse, error) {
			return h.subscribe(ctx, cmd, parts[1:])
		})
	case "unsubscribe":
		return h.requireAdmin(ctx, cmd, func() (*SlashResponse, error) {
			return h.unsubscribe(ctx, cmd, parts[1:])
		})
	case "filter":
		return h.requireAdmin(ctx, cmd, func() (*SlashResponse, error) {
			return h.addFilter(ctx, cmd, parts[1:])
		})
	case "filters":
		return h.listFilters(ctx, cmd)
	case "subscriptions":
		return h.listSubscriptions(ctx, cmd)
	case "setup":
		return h.requireAdmin(ctx, cmd, func() (*SlashResponse, error) {
			return h.setup(ctx, cmd)
		})
	case "set-suffix":
		return h.requireAdmin(ctx, cmd, func() (*SlashResponse, error) {
			return h.setSuffix(ctx, cmd, parts[1:])
		})
	case "set-link":
		return h.requireAdmin(ctx, cmd, func() (*SlashResponse, error) {
			return h.setLink(ctx, cmd, parts[1:])
		})
	case "admins":
		return h.requireAdmin(ctx, cmd, func() (*SlashResponse, error) {
			return h.admins(ctx, cmd, parts[1:])
		})
	case "api-key":
		return h.requireAdmin(ctx, cmd, func() (*SlashResponse, error) {
			return h.apiKey(ctx, cmd)
		})
	case "help":
		return h.help(ctx, cmd)
	default:
		return ephemeral(fmt.Sprintf("Unknown command: `%s`. Try `/zoom-notifier help`.", parts[0])), nil
	}
}

func (h *CommandHandler) requireAdmin(ctx context.Context, cmd SlashCommand, fn func() (*SlashResponse, error)) (*SlashResponse, error) {
	isAdmin, err := h.store.IsAdmin(ctx, cmd.TeamID, cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("check admin: %w", err)
	}
	if !isAdmin {
		return ephemeral("Permission denied. Only admins can run this command."), nil
	}
	return fn()
}

func (h *CommandHandler) status(ctx context.Context, cmd SlashCommand) (*SlashResponse, error) {
	meetings, err := h.store.ListActiveMeetings(ctx, cmd.TeamID)
	if err != nil {
		return nil, fmt.Errorf("list meetings: %w", err)
	}

	if len(meetings) == 0 {
		return ephemeral("No active meetings."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Active Meetings (%d):*\n", len(meetings)))
	for _, m := range meetings {
		participants, err := h.store.GetActiveParticipants(ctx, m.MeetingID)
		if err != nil {
			continue
		}
		topic := m.Topic
		if topic == "" {
			topic = "(no topic)"
		}
		sb.WriteString(fmt.Sprintf("• *%s* — %d participant(s)\n", topic, len(participants)))
		for _, p := range participants {
			sb.WriteString(fmt.Sprintf("    ◦ %s\n", p.UserName))
		}
	}
	return inChannel(sb.String()), nil
}

func (h *CommandHandler) whois(ctx context.Context, cmd SlashCommand, args []string) (*SlashResponse, error) {
	if len(args) == 0 {
		return ephemeral("Usage: `/zoom-notifier whois <meeting-topic>`"), nil
	}

	topic := strings.Join(args, " ")
	meetings, err := h.store.ListActiveMeetings(ctx, cmd.TeamID)
	if err != nil {
		return nil, fmt.Errorf("list meetings: %w", err)
	}

	for _, m := range meetings {
		if strings.EqualFold(m.Topic, topic) || m.MeetingID == topic {
			participants, err := h.store.GetActiveParticipants(ctx, m.MeetingID)
			if err != nil {
				return nil, fmt.Errorf("get participants: %w", err)
			}
			if len(participants) == 0 {
				return ephemeral(fmt.Sprintf("No active participants in *%s*.", m.Topic)), nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("*Participants in %s (%d):*\n", m.Topic, len(participants)))
			for _, p := range participants {
				sb.WriteString(fmt.Sprintf("• %s\n", p.UserName))
			}
			return ephemeral(sb.String()), nil
		}
	}

	return ephemeral(fmt.Sprintf("No active meeting found matching `%s`.", topic)), nil
}

func (h *CommandHandler) subscribe(ctx context.Context, cmd SlashCommand, args []string) (*SlashResponse, error) {
	if len(args) == 0 {
		return ephemeral("Usage: `/zoom-notifier subscribe #channel`"), nil
	}

	target := parseChannelID(args[0])
	sub := &store.Subscription{
		TenantID: cmd.TeamID,
		Type:     "slack",
		Target:   target,
		Enabled:  true,
	}
	if err := h.store.CreateSubscription(ctx, sub); err != nil {
		return nil, fmt.Errorf("create subscription: %w", err)
	}

	return ephemeral(fmt.Sprintf("Subscribed <#%s> to meeting notifications.\nRemember to invite the bot to the channel: `/invite @zoom-notifier`", target)), nil
}

func (h *CommandHandler) unsubscribe(ctx context.Context, cmd SlashCommand, args []string) (*SlashResponse, error) {
	if len(args) == 0 {
		return ephemeral("Usage: `/zoom-notifier unsubscribe #channel`"), nil
	}

	target := parseChannelID(args[0])
	subs, err := h.store.ListSubscriptions(ctx, cmd.TeamID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}

	for _, sub := range subs {
		if sub.Target == target && sub.Type == "slack" {
			if err := h.store.DeleteSubscription(ctx, sub.ID); err != nil {
				return nil, fmt.Errorf("delete subscription: %w", err)
			}
			return ephemeral(fmt.Sprintf("Unsubscribed %s from meeting notifications.", target)), nil
		}
	}

	return ephemeral(fmt.Sprintf("No Slack subscription found for %s.", target)), nil
}

func (h *CommandHandler) addFilter(ctx context.Context, cmd SlashCommand, args []string) (*SlashResponse, error) {
	if len(args) == 0 {
		return ephemeral("Usage: `/zoom-notifier filter \"Topic Name\"`"), nil
	}

	pattern := strings.Join(args, " ")
	// Strip surrounding quotes if present
	pattern = strings.Trim(pattern, "\"'")

	f := &store.MeetingFilter{
		TenantID: cmd.TeamID,
		Pattern:  pattern,
	}
	if err := h.store.CreateFilter(ctx, f); err != nil {
		return nil, fmt.Errorf("create filter: %w", err)
	}

	return ephemeral(fmt.Sprintf("Added meeting filter: `%s`.", pattern)), nil
}

func (h *CommandHandler) listFilters(ctx context.Context, cmd SlashCommand) (*SlashResponse, error) {
	filters, err := h.store.ListFilters(ctx, cmd.TeamID)
	if err != nil {
		return nil, fmt.Errorf("list filters: %w", err)
	}

	if len(filters) == 0 {
		return ephemeral("No meeting filters configured. All meetings generate notifications."), nil
	}

	var sb strings.Builder
	sb.WriteString("*Meeting Filters:*\n")
	for _, f := range filters {
		sb.WriteString(fmt.Sprintf("• `%s` (id: %d)\n", f.Pattern, f.ID))
	}
	return ephemeral(sb.String()), nil
}

func (h *CommandHandler) listSubscriptions(ctx context.Context, cmd SlashCommand) (*SlashResponse, error) {
	subs, err := h.store.ListSubscriptions(ctx, cmd.TeamID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}

	if len(subs) == 0 {
		return ephemeral("No subscriptions configured. Use `/zoom-notifier subscribe #channel` to add one."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Subscriptions (%d):*\n", len(subs)))
	for _, s := range subs {
		status := "enabled"
		if !s.Enabled {
			status = "disabled"
		}
		details := fmt.Sprintf("• %s → %s (%s)", s.Type, s.Target, status)
		if s.IncludeLink {
			details += " [links on]"
		}
		if s.MsgSuffix != "" {
			details += fmt.Sprintf(" suffix: `%s`", s.MsgSuffix)
		}
		sb.WriteString(details + "\n")
	}
	return ephemeral(sb.String()), nil
}

func (h *CommandHandler) setup(ctx context.Context, cmd SlashCommand) (*SlashResponse, error) {
	return ephemeral("To configure Zoom credentials, use the REST API:\n" +
		"```\n" +
		"curl -X PUT /api/v1/tenants/" + cmd.TeamID + "/zoom \\\n" +
		"  -H 'Authorization: Bearer <api-key>' \\\n" +
		"  -d '{\"client_id\": \"...\", \"client_secret\": \"...\", \"account_id\": \"...\"}'\n" +
		"```\n" +
		"You can get your API key with `/zoom-notifier api-key`."), nil
}

func (h *CommandHandler) setSuffix(ctx context.Context, cmd SlashCommand, args []string) (*SlashResponse, error) {
	if len(args) < 2 {
		return ephemeral("Usage: `/zoom-notifier set-suffix #channel \"the standup\"`"), nil
	}

	target := parseChannelID(args[0])
	suffix := strings.Join(args[1:], " ")
	suffix = strings.Trim(suffix, "\"'")

	subs, err := h.store.ListSubscriptions(ctx, cmd.TeamID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}

	for _, sub := range subs {
		if sub.Target == target && sub.Type == "slack" {
			sub.MsgSuffix = suffix
			if err := h.store.UpdateSubscription(ctx, sub); err != nil {
				return nil, fmt.Errorf("update subscription: %w", err)
			}
			return ephemeral(fmt.Sprintf("Updated message suffix for %s to `%s`.", target, suffix)), nil
		}
	}

	return ephemeral(fmt.Sprintf("No Slack subscription found for %s.", target)), nil
}

func (h *CommandHandler) setLink(ctx context.Context, cmd SlashCommand, args []string) (*SlashResponse, error) {
	if len(args) < 2 {
		return ephemeral("Usage: `/zoom-notifier set-link #channel on|off`"), nil
	}

	target := parseChannelID(args[0])
	var includeLink bool
	switch strings.ToLower(args[1]) {
	case "on", "true", "yes":
		includeLink = true
	case "off", "false", "no":
		includeLink = false
	default:
		return ephemeral("Usage: `/zoom-notifier set-link #channel on|off`"), nil
	}

	subs, err := h.store.ListSubscriptions(ctx, cmd.TeamID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}

	for _, sub := range subs {
		if sub.Target == target && sub.Type == "slack" {
			sub.IncludeLink = includeLink
			if err := h.store.UpdateSubscription(ctx, sub); err != nil {
				return nil, fmt.Errorf("update subscription: %w", err)
			}
			state := "disabled"
			if includeLink {
				state = "enabled"
			}
			return ephemeral(fmt.Sprintf("Meeting links %s for %s.", state, target)), nil
		}
	}

	return ephemeral(fmt.Sprintf("No Slack subscription found for %s.", target)), nil
}

func (h *CommandHandler) admins(ctx context.Context, cmd SlashCommand, args []string) (*SlashResponse, error) {
	if len(args) < 2 || args[0] != "add" {
		return ephemeral("Usage: `/zoom-notifier admins add @user`"), nil
	}

	userID := args[1]
	// Strip <@ > Slack mention formatting
	userID = strings.TrimPrefix(userID, "<@")
	userID = strings.TrimSuffix(userID, ">")
	// Handle <@U123|name> format
	if idx := strings.Index(userID, "|"); idx >= 0 {
		userID = userID[:idx]
	}

	if err := h.store.AddAdmin(ctx, cmd.TeamID, userID); err != nil {
		return nil, fmt.Errorf("add admin: %w", err)
	}

	return ephemeral(fmt.Sprintf("Added <@%s> as admin.", userID)), nil
}

func (h *CommandHandler) apiKey(ctx context.Context, cmd SlashCommand) (*SlashResponse, error) {
	tenant, err := h.store.GetTenant(ctx, cmd.TeamID)
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	if tenant == nil {
		return ephemeral("Tenant not found. Has the app been installed?"), nil
	}

	return ephemeral(fmt.Sprintf("Your API key (keep this secret!):\n```\n%s\n```", tenant.APIKey)), nil
}

func (h *CommandHandler) help(ctx context.Context, cmd SlashCommand) (*SlashResponse, error) {
	text := "*zoom-notifier commands:*\n" +
		"• `/zoom-notifier status` — Show active meetings\n" +
		"• `/zoom-notifier whois <meeting>` — List participants in a meeting\n" +
		"• `/zoom-notifier subscribe #channel` — Subscribe channel to notifications _(admin)_\n" +
		"• `/zoom-notifier unsubscribe #channel` — Unsubscribe channel _(admin)_\n" +
		"• `/zoom-notifier filter \"Topic\"` — Add a meeting topic filter _(admin)_\n" +
		"• `/zoom-notifier filters` — List active filters\n" +
		"• `/zoom-notifier subscriptions` — List channel subscriptions\n" +
		"• `/zoom-notifier set-suffix #channel \"text\"` — Set message suffix _(admin)_\n" +
		"• `/zoom-notifier set-link #channel on|off` — Toggle meeting links _(admin)_\n" +
		"• `/zoom-notifier admins add @user` — Add an admin _(admin)_\n" +
		"• `/zoom-notifier api-key` — Show tenant API key _(admin)_\n" +
		"• `/zoom-notifier setup` — Zoom credential setup instructions _(admin)_\n" +
		"• `/zoom-notifier help` — Show this help"
	return ephemeral(text), nil
}
