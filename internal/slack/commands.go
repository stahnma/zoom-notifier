package slack

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	slackapi "github.com/slack-go/slack"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

// SlashCommand represents a parsed Slack slash command.
type SlashCommand struct {
	TeamID    string
	UserID    string
	ChannelID string
	Text      string
	TriggerID string
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
	s = strings.TrimPrefix(s, "#")
	return s
}

// parseChannelName extracts the channel name from Slack's mention format.
// e.g. "<#C05MXNTTHM4|bottery-new>" returns "bottery-new".
// Returns empty string if no name is present.
func parseChannelName(s string) string {
	if !strings.HasPrefix(s, "<#") {
		return ""
	}
	s = strings.TrimPrefix(s, "<#")
	s = strings.TrimSuffix(s, ">")
	if idx := strings.Index(s, "|"); idx >= 0 {
		return s[idx+1:]
	}
	return ""
}

// formatChannel returns a Slack-linkable channel reference.
// If target looks like a channel ID (e.g. C05MXNTTHM4), wraps it as <#ID>.
// Otherwise returns #name for display.
func formatChannel(target string) string {
	if strings.HasPrefix(target, "C") && len(target) > 5 {
		return "<#" + target + ">"
	}
	return "#" + target
}

// CommandHandler routes slash commands to their implementations.
type CommandHandler struct {
	store     store.Store
	modals    ModalOpener
	serverURL string
}

func NewCommandHandler(s store.Store) *CommandHandler {
	return &CommandHandler{store: s}
}

// SetModalOpener sets the ModalOpener used to open Slack modals.
// If nil, commands fall back to text-based parsing.
func (h *CommandHandler) SetModalOpener(m ModalOpener) {
	h.modals = m
}

// SetServerURL sets the base URL used to build setup links.
func (h *CommandHandler) SetServerURL(url string) {
	h.serverURL = url
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
	case "subscribe", "sub":
		return h.requireAdmin(ctx, cmd, func() (*SlashResponse, error) {
			return h.subscribe(ctx, cmd, parts[1:])
		})
	case "unsubscribe", "unsub":
		return h.requireAdmin(ctx, cmd, func() (*SlashResponse, error) {
			return h.unsubscribe(ctx, cmd, parts[1:])
		})
	case "filter":
		return h.requireAdmin(ctx, cmd, func() (*SlashResponse, error) {
			return h.addFilter(ctx, cmd, parts[1:])
		})
	case "filters":
		return h.listFilters(ctx, cmd)
	case "subscriptions", "subs":
		return h.requireAdmin(ctx, cmd, func() (*SlashResponse, error) {
			return h.listSubscriptions(ctx, cmd)
		})
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
	case "settings":
		return h.settings(ctx, cmd)
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

	log.WithFields(log.Fields{
		"team_id":         cmd.TeamID,
		"active_meetings": len(meetings),
	}).Debug("status command")

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
		return ephemeral("Usage: `/zoom-notifier whois <search>` — search by meeting topic (partial match)"), nil
	}

	topic := strings.Join(args, " ")
	meetings, err := h.store.ListActiveMeetings(ctx, cmd.TeamID)
	if err != nil {
		return nil, fmt.Errorf("list meetings: %w", err)
	}

	// Strip surrounding quotes if present (from copy-paste)
	topic = strings.Trim(topic, "\"'\u201c\u201d")

	log.WithFields(log.Fields{
		"team_id": cmd.TeamID,
		"search":  topic,
	}).Debug("whois command")

	var matched []*store.ActiveMeeting
	for _, m := range meetings {
		if strings.EqualFold(m.Topic, topic) || m.MeetingID == topic ||
			strings.Contains(strings.ToLower(m.Topic), strings.ToLower(topic)) {
			matched = append(matched, m)
		}
	}

	log.WithFields(log.Fields{
		"team_id": cmd.TeamID,
		"search":  topic,
		"matched": len(matched),
	}).Debug("whois search results")

	if len(matched) == 0 {
		return ephemeral(fmt.Sprintf("No active meeting found matching `%s`.", topic)), nil
	}

	var sb strings.Builder
	for _, m := range matched {
		participants, err := h.store.GetActiveParticipants(ctx, m.MeetingID)
		if err != nil {
			return nil, fmt.Errorf("get participants: %w", err)
		}
		if len(participants) == 0 {
			sb.WriteString(fmt.Sprintf("No active participants in *%s*.\n", m.Topic))
			continue
		}
		sb.WriteString(fmt.Sprintf("*Participants in %s (%d):*\n", m.Topic, len(participants)))
		for _, p := range participants {
			sb.WriteString(fmt.Sprintf("• %s\n", p.UserName))
		}
	}
	return ephemeral(sb.String()), nil
}

func (h *CommandHandler) subscribe(ctx context.Context, cmd SlashCommand, args []string) (*SlashResponse, error) {
	// Open modal if no args and modal support is available
	if len(args) == 0 && h.canOpenModal(cmd) {
		botToken := h.getBotToken(ctx, cmd.TeamID)
		if botToken != "" {
			modal := BuildSubscribeModal()
			modal.PrivateMetadata = cmd.TeamID + "|" + cmd.ChannelID
			opener := NewSlackModalOpener(botToken)
			if _, err := opener.OpenView(cmd.TriggerID, modal); err != nil {
				log.WithError(err).Error("failed to open subscribe modal")
			} else {
				return ephemeral(""), nil
			}
		}
	}

	if len(args) == 0 {
		return ephemeral("Usage: `/zoom-notifier subscribe #channel`"), nil
	}

	target := parseChannelID(args[0])

	log.WithFields(log.Fields{
		"team_id": cmd.TeamID,
		"channel": target,
	}).Debug("subscribe command")

	// Check for duplicate subscription
	existing, err := h.store.ListSubscriptions(ctx, cmd.TeamID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	for _, s := range existing {
		if s.Target == target && s.Type == "slack" {
			return ephemeral(fmt.Sprintf("%s is already subscribed to meeting notifications.", formatChannel(target))), nil
		}
	}

	sub := &store.Subscription{
		TenantID: cmd.TeamID,
		Type:     "slack",
		Target:   target,
		Enabled:  true,
	}
	if err := h.store.CreateSubscription(ctx, sub); err != nil {
		return nil, fmt.Errorf("create subscription: %w", err)
	}

	return ephemeral(fmt.Sprintf("Subscribed %s to meeting notifications.", formatChannel(target))), nil
}

func (h *CommandHandler) unsubscribe(ctx context.Context, cmd SlashCommand, args []string) (*SlashResponse, error) {
	if len(args) == 0 && h.canOpenModal(cmd) {
		botToken := h.getBotToken(ctx, cmd.TeamID)
		if botToken != "" {
			subs, err := h.store.ListSubscriptions(ctx, cmd.TeamID)
			if err == nil && len(subs) > 0 {
				modal := BuildUnsubscribeModal(subs)
				modal.PrivateMetadata = cmd.TeamID + "|" + cmd.ChannelID
				opener := NewSlackModalOpener(botToken)
				if _, err := opener.OpenView(cmd.TriggerID, modal); err != nil {
					log.WithError(err).Error("failed to open unsubscribe modal")
				} else {
					return ephemeral(""), nil
				}
			}
		}
	}

	if len(args) == 0 {
		return ephemeral("Usage: `/zoom-notifier unsubscribe #channel`"), nil
	}

	target := parseChannelID(args[0])
	targetName := parseChannelName(args[0])

	log.WithFields(log.Fields{
		"team_id":      cmd.TeamID,
		"channel":      target,
		"channel_name": targetName,
	}).Debug("unsubscribe command")

	subs, err := h.store.ListSubscriptions(ctx, cmd.TeamID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}

	// Build a map of channel ID → name for matching
	botToken := h.getBotToken(ctx, cmd.TeamID)
	var api *slackapi.Client
	if botToken != "" {
		api = slackapi.New(botToken)
	}

	for _, sub := range subs {
		if sub.Type != "slack" {
			continue
		}

		// Direct match
		match := sub.Target == target || (targetName != "" && sub.Target == targetName)

		// If no direct match and sub is stored as a channel ID, resolve to name and compare
		if !match && api != nil && strings.HasPrefix(sub.Target, "C") {
			info, err := api.GetConversationInfoContext(ctx, &slackapi.GetConversationInfoInput{ChannelID: sub.Target})
			if err == nil && info != nil {
				match = info.Name == target || (targetName != "" && info.Name == targetName)
			}
		}

		if match {
			if err := h.store.DeleteSubscription(ctx, sub.ID); err != nil {
				return nil, fmt.Errorf("delete subscription: %w", err)
			}
			return ephemeral(fmt.Sprintf("Unsubscribed %s from meeting notifications.", formatChannel(sub.Target))), nil
		}
	}

	return ephemeral(fmt.Sprintf("No Slack subscription found for %s.", formatChannel(target))), nil
}

func (h *CommandHandler) addFilter(ctx context.Context, cmd SlashCommand, args []string) (*SlashResponse, error) {
	// Open modal if no args and modal support is available
	if len(args) == 0 && h.canOpenModal(cmd) {
		botToken := h.getBotToken(ctx, cmd.TeamID)
		if botToken != "" {
			modal := BuildFilterModal()
			modal.PrivateMetadata = cmd.TeamID + "|" + cmd.ChannelID
			opener := NewSlackModalOpener(botToken)
			if _, err := opener.OpenView(cmd.TriggerID, modal); err != nil {
				log.WithError(err).Error("failed to open filter modal")
			} else {
				return ephemeral(""), nil
			}
		}
	}

	if len(args) == 0 {
		return ephemeral("Usage: `/zoom-notifier filter \"Topic Name\"`"), nil
	}

	pattern := strings.Join(args, " ")
	// Strip surrounding quotes if present
	pattern = strings.Trim(pattern, "\"'")

	log.WithFields(log.Fields{
		"team_id": cmd.TeamID,
		"pattern": pattern,
	}).Debug("add filter command")

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

	log.WithFields(log.Fields{
		"team_id": cmd.TeamID,
		"count":   len(filters),
	}).Debug("list filters command")

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

	log.WithFields(log.Fields{
		"team_id": cmd.TeamID,
		"count":   len(subs),
	}).Debug("list subscriptions command")

	if len(subs) == 0 {
		return ephemeral("No subscriptions configured. Use `/zoom-notifier subscribe #channel` to add one."), nil
	}

	// Resolve channel IDs/names via Slack API for display
	channelIDs := make(map[string]string) // target -> channel ID
	botToken := h.getBotToken(ctx, cmd.TeamID)
	if botToken != "" {
		api := slackapi.New(botToken)
		for _, s := range subs {
			if s.Type != "slack" {
				continue
			}
			if strings.HasPrefix(s.Target, "C") {
				// Already a channel ID
				channelIDs[s.Target] = s.Target
			} else {
				// Name-based target — search for matching channel
				channels, _, err := api.GetConversationsContext(ctx, &slackapi.GetConversationsParameters{
					Types:           []string{"public_channel", "private_channel"},
					ExcludeArchived: true,
					Limit:           200,
				})
				if err == nil {
					for _, ch := range channels {
						if ch.Name == s.Target {
							channelIDs[s.Target] = ch.ID
							break
						}
					}
				}
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Subscriptions (%d):*\n", len(subs)))
	for _, s := range subs {
		status := "enabled"
		if !s.Enabled {
			status = "disabled"
		}
		target := s.Target
		if id, ok := channelIDs[s.Target]; ok {
			target = "<#" + id + ">"
		} else {
			target = formatChannel(s.Target)
		}
		details := fmt.Sprintf("• %s → %s (%s)", s.Type, target, status)
		sb.WriteString(details + "\n")
	}
	return ephemeral(sb.String()), nil
}

func (h *CommandHandler) setup(ctx context.Context, cmd SlashCommand) (*SlashResponse, error) {
	tenant, err := h.store.GetTenant(ctx, cmd.TeamID)
	if err != nil || tenant == nil {
		return ephemeral("Tenant not found. Has the app been installed?"), nil
	}
	setupURL := h.serverURL + "/tenant/setup?key=" + tenant.APIKey
	return ephemeral(fmt.Sprintf(
		"Configure Zoom credentials for this workspace:\n<%s|Open Zoom Setup>\n\n"+
			"Keep this link private — it contains your API key.",
		setupURL,
	)), nil
}

// parseQuotedArgs splits args respecting quoted strings.
// e.g. ["\"Daily", "Standup\"", "\"the", "standup\""] -> ["Daily Standup", "the standup"]
func parseQuotedArgs(args []string) []string {
	raw := strings.Join(args, " ")
	var result []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if !inQuote {
			if ch == '"' || ch == '\'' {
				inQuote = true
				quoteChar = ch
				current.Reset()
			} else if ch == ' ' {
				if current.Len() > 0 {
					result = append(result, current.String())
					current.Reset()
				}
			} else {
				current.WriteByte(ch)
			}
		} else {
			if ch == quoteChar {
				result = append(result, current.String())
				current.Reset()
				inQuote = false
			} else {
				current.WriteByte(ch)
			}
		}
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}

// canOpenModal returns true if the command has a trigger_id and the handler has a modal opener.
func (h *CommandHandler) canOpenModal(cmd SlashCommand) bool {
	return h.modals != nil && cmd.TriggerID != ""
}

// getBotToken fetches the bot token for a tenant. Returns empty string if unavailable.
func (h *CommandHandler) getBotToken(ctx context.Context, teamID string) string {
	tenant, err := h.store.GetTenant(ctx, teamID)
	if err != nil || tenant == nil || tenant.BotToken == nil || *tenant.BotToken == "" {
		return ""
	}
	return *tenant.BotToken
}

func (h *CommandHandler) setSuffix(ctx context.Context, cmd SlashCommand, args []string) (*SlashResponse, error) {
	// Open modal if no args and modal support is available
	if len(args) == 0 && h.canOpenModal(cmd) {
		log.WithField("trigger_id", cmd.TriggerID).Debug("attempting to open set-suffix modal")
		botToken := h.getBotToken(ctx, cmd.TeamID)
		if botToken != "" {
			tenant, err := h.store.GetTenant(ctx, cmd.TeamID)
			if err != nil {
				return nil, fmt.Errorf("get tenant: %w", err)
			}
			filters, err := h.store.ListFilters(ctx, cmd.TeamID)
			if err != nil {
				return nil, fmt.Errorf("list filters: %w", err)
			}
			modal := BuildSetSuffixModal(tenant.DefaultMsgSuffix, filters)
			modal.PrivateMetadata = cmd.TeamID + "|" + cmd.ChannelID
			opener := NewSlackModalOpener(botToken)
			if _, err := opener.OpenView(cmd.TriggerID, modal); err != nil {
				log.WithError(err).Error("failed to open set-suffix modal")
			} else {
				log.Debug("set-suffix modal opened successfully")
				return ephemeral(""), nil
			}
		} else {
			log.Warn("no bot token available, falling back to text command")
		}
	}

	if len(args) == 0 {
		return ephemeral("Usage:\n" +
			"• `/zoom-notifier set-suffix the zoom meeting.` — set tenant-wide default suffix\n" +
			"• `/zoom-notifier set-suffix \"Filter\" \"text\"` — set suffix override on a filter"), nil
	}

	raw := strings.Join(args, " ")
	parsed := parseQuotedArgs(args)

	// Filter override: requires 2+ quoted args (e.g. "Standup" "the standup.")
	hasQuotes := strings.ContainsAny(raw, "\"'")
	if hasQuotes && len(parsed) >= 2 {
		pattern := parsed[0]
		suffix := strings.Join(parsed[1:], " ")
		filter, err := h.findFilterByPattern(ctx, cmd.TeamID, pattern)
		if err != nil {
			return nil, err
		}
		if filter == nil {
			return ephemeral(fmt.Sprintf("No filter found matching `%s`. Use `/zoom-notifier filters` to see available filters.", pattern)), nil
		}
		filter.MsgSuffix = &suffix
		if err := h.store.UpdateFilter(ctx, filter); err != nil {
			return nil, fmt.Errorf("update filter: %w", err)
		}
		log.WithFields(log.Fields{
			"team_id": cmd.TeamID,
			"filter":  pattern,
			"suffix":  suffix,
		}).Debug("set suffix on filter via command")
		return ephemeral(fmt.Sprintf("Updated suffix override on filter `%s` to `%s`.", pattern, suffix)), nil
	}

	// Tenant-wide default: unquoted text or single quoted arg
	suffix := strings.Trim(raw, "\"' ")
	tenant, err := h.store.GetTenant(ctx, cmd.TeamID)
	if err != nil || tenant == nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	if err := h.store.UpdateTenantDefaults(ctx, cmd.TeamID, suffix, tenant.DefaultIncludeLink); err != nil {
		return nil, fmt.Errorf("update tenant defaults: %w", err)
	}
	log.WithFields(log.Fields{
		"team_id": cmd.TeamID,
		"suffix":  suffix,
	}).Debug("set tenant default suffix via command")
	return ephemeral(fmt.Sprintf("Updated default message suffix to `%s`.", suffix)), nil
}

func (h *CommandHandler) setLink(ctx context.Context, cmd SlashCommand, args []string) (*SlashResponse, error) {
	// Open modal if no args and modal support is available
	if len(args) == 0 && h.canOpenModal(cmd) {
		botToken := h.getBotToken(ctx, cmd.TeamID)
		if botToken != "" {
			tenant, err := h.store.GetTenant(ctx, cmd.TeamID)
			if err != nil {
				return nil, fmt.Errorf("get tenant: %w", err)
			}
			filters, err := h.store.ListFilters(ctx, cmd.TeamID)
			if err != nil {
				return nil, fmt.Errorf("list filters: %w", err)
			}
			modal := BuildSetLinkModal(tenant.DefaultIncludeLink, filters)
			modal.PrivateMetadata = cmd.TeamID + "|" + cmd.ChannelID
			opener := NewSlackModalOpener(botToken)
			if _, err := opener.OpenView(cmd.TriggerID, modal); err != nil {
				log.WithError(err).Error("failed to open set-link modal")
			} else {
				return ephemeral(""), nil
			}
		}
	}

	if len(args) == 0 {
		return ephemeral("Usage:\n" +
			"• `/zoom-notifier set-link on|off` — set tenant-wide default\n" +
			"• `/zoom-notifier set-link \"Filter\" on|off` — set override on a filter"), nil
	}

	parseBool := func(s string) (bool, bool) {
		switch strings.ToLower(s) {
		case "on", "true", "yes":
			return true, true
		case "off", "false", "no":
			return false, true
		default:
			return false, false
		}
	}

	if len(args) == 1 {
		// Tenant-wide default
		includeLink, ok := parseBool(args[0])
		if !ok {
			return ephemeral("Usage: `/zoom-notifier set-link on|off`"), nil
		}
		tenant, err := h.store.GetTenant(ctx, cmd.TeamID)
		if err != nil || tenant == nil {
			return nil, fmt.Errorf("get tenant: %w", err)
		}
		if err := h.store.UpdateTenantDefaults(ctx, cmd.TeamID, tenant.DefaultMsgSuffix, includeLink); err != nil {
			return nil, fmt.Errorf("update tenant defaults: %w", err)
		}
		state := "disabled"
		if includeLink {
			state = "enabled"
		}
		log.WithFields(log.Fields{
			"team_id":      cmd.TeamID,
			"include_link": state,
		}).Debug("set tenant default link via command")
		return ephemeral(fmt.Sprintf("Meeting links %s (tenant-wide default).", state)), nil
	}

	// Filter override: last arg is on/off, everything before is the filter pattern
	lastArg := args[len(args)-1]
	includeLink, ok := parseBool(lastArg)
	if !ok {
		return ephemeral("Usage: `/zoom-notifier set-link \"Filter\" on|off`"), nil
	}
	parsed := parseQuotedArgs(args[:len(args)-1])
	if len(parsed) == 0 {
		return ephemeral("Usage: `/zoom-notifier set-link \"Filter\" on|off`"), nil
	}
	pattern := parsed[0]
	filter, err := h.findFilterByPattern(ctx, cmd.TeamID, pattern)
	if err != nil {
		return nil, err
	}
	if filter == nil {
		return ephemeral(fmt.Sprintf("No filter found matching `%s`. Use `/zoom-notifier filters` to see available filters.", pattern)), nil
	}
	filter.IncludeLink = &includeLink
	if err := h.store.UpdateFilter(ctx, filter); err != nil {
		return nil, fmt.Errorf("update filter: %w", err)
	}
	state := "disabled"
	if includeLink {
		state = "enabled"
	}
	log.WithFields(log.Fields{
		"team_id":      cmd.TeamID,
		"filter":       pattern,
		"include_link": state,
	}).Debug("set link on filter via command")
	return ephemeral(fmt.Sprintf("Meeting links %s for filter `%s`.", state, pattern)), nil
}

func (h *CommandHandler) findFilterByPattern(ctx context.Context, tenantID, pattern string) (*store.MeetingFilter, error) {
	filters, err := h.store.ListFilters(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list filters: %w", err)
	}
	for _, f := range filters {
		if strings.EqualFold(f.Pattern, pattern) {
			return f, nil
		}
	}
	return nil, nil
}

func (h *CommandHandler) settings(ctx context.Context, cmd SlashCommand) (*SlashResponse, error) {
	log.WithField("team_id", cmd.TeamID).Debug("settings command")

	tenant, err := h.store.GetTenant(ctx, cmd.TeamID)
	if err != nil || tenant == nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("*Notification Settings:*\n\n")

	// Tenant defaults
	sb.WriteString("*Tenant Defaults:*\n")
	suffix := tenant.DefaultMsgSuffix
	if suffix == "" {
		suffix = "(none)"
	}
	linkState := "off"
	if tenant.DefaultIncludeLink {
		linkState = "on"
	}
	sb.WriteString(fmt.Sprintf("• Message suffix: `%s`\n", suffix))
	sb.WriteString(fmt.Sprintf("• Include meeting link: %s\n", linkState))

	// Filter overrides
	filters, err := h.store.ListFilters(ctx, cmd.TeamID)
	if err != nil {
		return nil, fmt.Errorf("list filters: %w", err)
	}

	if len(filters) > 0 {
		sb.WriteString("\n*Filter Overrides:*\n")
		for _, f := range filters {
			sb.WriteString(fmt.Sprintf("• `%s`", f.Pattern))
			var overrides []string
			if f.MsgSuffix != nil {
				overrides = append(overrides, fmt.Sprintf("suffix: `%s`", *f.MsgSuffix))
			}
			if f.IncludeLink != nil {
				link := "off"
				if *f.IncludeLink {
					link = "on"
				}
				overrides = append(overrides, fmt.Sprintf("link: %s", link))
			}
			if len(overrides) > 0 {
				sb.WriteString(fmt.Sprintf(" — %s", strings.Join(overrides, ", ")))
			} else {
				sb.WriteString(" — no overrides")
			}
			sb.WriteString("\n")
		}
	}

	// Show subscriptions for admins
	isAdmin, err := h.store.IsAdmin(ctx, cmd.TeamID, cmd.UserID)
	if err == nil && isAdmin {
		subs, err := h.store.ListSubscriptions(ctx, cmd.TeamID)
		if err == nil && len(subs) > 0 {
			sb.WriteString(fmt.Sprintf("\n*Subscriptions (%d):*\n", len(subs)))

			botToken := h.getBotToken(ctx, cmd.TeamID)
			var api *slackapi.Client
			if botToken != "" {
				api = slackapi.New(botToken)
			}

			for _, s := range subs {
				if s.Type != "slack" {
					sb.WriteString(fmt.Sprintf("• %s → %s\n", s.Type, s.Target))
					continue
				}
				target := formatChannel(s.Target)
				if api != nil && strings.HasPrefix(s.Target, "C") {
					target = "<#" + s.Target + ">"
				}
				sb.WriteString(fmt.Sprintf("• %s\n", target))
			}
		}
	}

	return ephemeral(sb.String()), nil
}

func parseUserID(s string) string {
	s = strings.TrimPrefix(s, "<@")
	s = strings.TrimSuffix(s, ">")
	if idx := strings.Index(s, "|"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimPrefix(s, "@")
	return s
}

func (h *CommandHandler) admins(ctx context.Context, cmd SlashCommand, args []string) (*SlashResponse, error) {
	if len(args) < 2 || (args[0] != "add" && args[0] != "remove") {
		return ephemeral("Usage:\n" +
			"• `/zoom-notifier admins add @user`\n" +
			"• `/zoom-notifier admins remove @user`"), nil
	}

	userID := parseUserID(args[1])

	switch args[0] {
	case "add":
		if err := h.store.AddAdmin(ctx, cmd.TeamID, userID); err != nil {
			return nil, fmt.Errorf("add admin: %w", err)
		}
		log.WithFields(log.Fields{
			"team_id":  cmd.TeamID,
			"admin_id": userID,
		}).Info("added admin via command")
		return ephemeral(fmt.Sprintf("Added <@%s> as admin.", userID)), nil

	case "remove":
		if userID == cmd.UserID {
			return ephemeral("You can't remove yourself as admin."), nil
		}
		if err := h.store.RemoveAdmin(ctx, cmd.TeamID, userID); err != nil {
			return nil, fmt.Errorf("remove admin: %w", err)
		}
		log.WithFields(log.Fields{
			"team_id":  cmd.TeamID,
			"admin_id": userID,
		}).Info("removed admin via command")
		return ephemeral(fmt.Sprintf("Removed <@%s> as admin.", userID)), nil
	}

	return ephemeral("Usage:\n" +
		"• `/zoom-notifier admins add @user`\n" +
		"• `/zoom-notifier admins remove @user`"), nil
}

func (h *CommandHandler) apiKey(ctx context.Context, cmd SlashCommand) (*SlashResponse, error) {
	log.WithField("team_id", cmd.TeamID).Debug("api-key command requested")

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
	isAdminUser, _ := h.store.IsAdmin(ctx, cmd.TeamID, cmd.UserID)
	log.WithFields(log.Fields{
		"team_id":  cmd.TeamID,
		"is_admin": isAdminUser,
	}).Debug("help command")

	text := "*zoom-notifier commands:*\n" +
		"• `/zoom-notifier status` — Show active meetings\n" +
		"• `/zoom-notifier whois <search>` — List participants by topic (partial match)\n" +
		"• `/zoom-notifier filters` — List active filters\n" +
		"• `/zoom-notifier settings` — Show notification settings and filter overrides\n" +
		"• `/zoom-notifier help` — Show this help"

	isAdmin, err := h.store.IsAdmin(ctx, cmd.TeamID, cmd.UserID)
	if err == nil && isAdmin {
		text += "\n\n*Admin commands:*\n" +
			"• `/zoom-notifier subscriptions` — List channel subscriptions\n" +
			"• `/zoom-notifier subscribe #channel` — Subscribe channel to notifications\n" +
			"• `/zoom-notifier unsubscribe #channel` — Unsubscribe channel\n" +
			"• `/zoom-notifier filter \"Topic\"` — Add a meeting topic filter\n" +
			"• `/zoom-notifier set-suffix \"text\"` — Set default message suffix\n" +
			"• `/zoom-notifier set-suffix \"Filter\" \"text\"` — Set suffix on a filter\n" +
			"• `/zoom-notifier set-link on|off` — Toggle default meeting links\n" +
			"• `/zoom-notifier set-link \"Filter\" on|off` — Toggle links on a filter\n" +
			"• `/zoom-notifier admins add @user` — Add an admin\n" +
			"• `/zoom-notifier admins remove @user` — Remove an admin\n" +
			"• `/zoom-notifier api-key` — Show tenant API key\n" +
			"• `/zoom-notifier setup` — Zoom credential setup instructions"
	}

	return ephemeral(text), nil
}
