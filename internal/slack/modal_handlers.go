package slack

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	slackapi "github.com/slack-go/slack"
	"github.com/stahnma/zoom-notifier/internal/store"
)

// parseMetadata extracts teamID and channelID from the modal's PrivateMetadata field.
func parseMetadata(metadata string) (teamID, channelID string) {
	parts := strings.SplitN(metadata, "|", 2)
	teamID = parts[0]
	if len(parts) > 1 {
		channelID = parts[1]
	}
	return
}

// postConfirmation sends an ephemeral message to the user in the channel where
// the slash command was invoked.
func (h *CommandHandler) postConfirmation(ctx context.Context, teamID, channelID, userID, text string) {
	botToken := h.getBotToken(ctx, teamID)
	if botToken == "" {
		return
	}
	if channelID == "" {
		channelID = userID // fall back to DM
	}
	api := slackapi.New(botToken)
	_, err := api.PostEphemeralContext(ctx, channelID, userID, slackapi.MsgOptionText(text, false))
	if err != nil {
		log.WithError(err).Debug("failed to send modal confirmation")
	} else {
		log.WithFields(log.Fields{
			"team_id":    teamID,
			"channel_id": channelID,
			"user_id":    userID,
		}).Debug("sent modal confirmation")
	}
}

// RegisterModalHandlers registers all modal submission handlers on the InteractionHandler.
func (h *CommandHandler) RegisterModalHandlers(ih *InteractionHandler) {
	ih.RegisterHandler("set_suffix", h.handleSetSuffixSubmission)
	ih.RegisterHandler("set_link", h.handleSetLinkSubmission)
	ih.RegisterHandler("subscribe", h.handleSubscribeSubmission)
	ih.RegisterHandler("unsubscribe", h.handleUnsubscribeSubmission)
	ih.RegisterHandler("admin_add", h.handleAdminAddSubmission)
	ih.RegisterHandler("add_filter", h.handleAddFilterSubmission)
}

func (h *CommandHandler) handleSetSuffixSubmission(payload InteractionPayload) error {
	ctx := context.Background()
	teamID, channelID := parseMetadata(payload.View.PrivateMetadata)
	if teamID == "" {
		teamID = payload.Team.ID
	}

	if payload.View.State == nil {
		return fmt.Errorf("no view state in payload")
	}

	filterValue := ""
	if fv, ok := payload.View.State.Values["filter_block"]["filter_select"]; ok && fv.Selected != nil {
		filterValue = fv.Selected.Value
	}

	suffixValue := ""
	if sv, ok := payload.View.State.Values["suffix_block"]["suffix_input"]; ok && sv.Value != nil {
		suffixValue = *sv.Value
	}

	log.WithFields(log.Fields{
		"team_id": teamID,
		"filter":  filterValue,
		"suffix":  suffixValue,
	}).Debug("set_suffix modal submission")

	if filterValue == "" || filterValue == "all" {
		tenant, err := h.store.GetTenant(ctx, teamID)
		if err != nil || tenant == nil {
			return fmt.Errorf("get tenant: %w", err)
		}
		if err := h.store.UpdateTenantDefaults(ctx, teamID, suffixValue, tenant.DefaultIncludeLink); err != nil {
			return err
		}
		log.WithFields(log.Fields{
			"team_id": teamID,
			"suffix":  suffixValue,
		}).Info("updated tenant default message suffix via modal")
		h.postConfirmation(ctx, teamID, channelID, payload.User.ID, fmt.Sprintf("Updated default message suffix to `%s`.", suffixValue))
		return nil
	}

	filter, err := h.getFilterBySelectValue(ctx, teamID, filterValue)
	if err != nil {
		return err
	}
	if filter == nil {
		return fmt.Errorf("filter not found: %s", filterValue)
	}
	filter.MsgSuffix = &suffixValue
	if err := h.store.UpdateFilter(ctx, filter); err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"team_id": teamID,
		"filter":  filter.Pattern,
		"suffix":  suffixValue,
	}).Info("updated filter suffix override via modal")
	h.postConfirmation(ctx, teamID, channelID, payload.User.ID, fmt.Sprintf("Updated suffix on filter `%s` to `%s`.", filter.Pattern, suffixValue))
	return nil
}

func (h *CommandHandler) handleSetLinkSubmission(payload InteractionPayload) error {
	ctx := context.Background()
	teamID, channelID := parseMetadata(payload.View.PrivateMetadata)
	if teamID == "" {
		teamID = payload.Team.ID
	}

	if payload.View.State == nil {
		return fmt.Errorf("no view state in payload")
	}

	filterValue := ""
	if fv, ok := payload.View.State.Values["filter_block"]["filter_select"]; ok && fv.Selected != nil {
		filterValue = fv.Selected.Value
	}

	linkValue := "off"
	if lv, ok := payload.View.State.Values["link_block"]["link_input"]; ok && lv.Selected != nil {
		linkValue = lv.Selected.Value
	}
	includeLink := linkValue == "on"

	log.WithFields(log.Fields{
		"team_id": teamID,
		"filter":  filterValue,
		"link":    linkValue,
	}).Debug("set_link modal submission")

	state := "off"
	if includeLink {
		state = "on"
	}

	if filterValue == "" || filterValue == "all" {
		tenant, err := h.store.GetTenant(ctx, teamID)
		if err != nil || tenant == nil {
			return fmt.Errorf("get tenant: %w", err)
		}
		if err := h.store.UpdateTenantDefaults(ctx, teamID, tenant.DefaultMsgSuffix, includeLink); err != nil {
			return err
		}
		log.WithFields(log.Fields{
			"team_id":      teamID,
			"include_link": state,
		}).Info("updated tenant default meeting link setting via modal")
		h.postConfirmation(ctx, teamID, channelID, payload.User.ID, fmt.Sprintf("Meeting links set to %s (default).", state))
		return nil
	}

	filter, err := h.getFilterBySelectValue(ctx, teamID, filterValue)
	if err != nil {
		return err
	}
	if filter == nil {
		return fmt.Errorf("filter not found: %s", filterValue)
	}
	filter.IncludeLink = &includeLink
	if err := h.store.UpdateFilter(ctx, filter); err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"team_id":      teamID,
		"filter":       filter.Pattern,
		"include_link": state,
	}).Info("updated filter link override via modal")
	h.postConfirmation(ctx, teamID, channelID, payload.User.ID, fmt.Sprintf("Meeting links set to %s on filter `%s`.", state, filter.Pattern))
	return nil
}

func (h *CommandHandler) handleSubscribeSubmission(payload InteractionPayload) error {
	ctx := context.Background()
	teamID, channelID := parseMetadata(payload.View.PrivateMetadata)
	if teamID == "" {
		teamID = payload.Team.ID
	}

	if payload.View.State == nil {
		return fmt.Errorf("no view state in payload")
	}

	selectedChannel := ""
	if cv, ok := payload.View.State.Values["channel_block"]["channel_select"]; ok {
		if cv.SelectedConversation != nil {
			selectedChannel = *cv.SelectedConversation
		}
	}
	if selectedChannel == "" {
		return fmt.Errorf("no channel selected")
	}

	log.WithFields(log.Fields{
		"team_id": teamID,
		"channel": selectedChannel,
	}).Debug("subscribe modal submission")

	// Check for duplicate
	existing, err := h.store.ListSubscriptions(ctx, teamID)
	if err != nil {
		return fmt.Errorf("list subscriptions: %w", err)
	}
	for _, s := range existing {
		if s.Target == selectedChannel && s.Type == "slack" {
			h.postConfirmation(ctx, teamID, channelID, payload.User.ID, fmt.Sprintf("<#%s> is already subscribed.", selectedChannel))
			return nil
		}
	}

	sub := &store.Subscription{
		TenantID: teamID,
		Type:     "slack",
		Target:   selectedChannel,
		Enabled:  true,
	}
	if err := h.store.CreateSubscription(ctx, sub); err != nil {
		return err
	}

	h.postConfirmation(ctx, teamID, channelID, payload.User.ID, fmt.Sprintf("Subscribed <#%s> to meeting notifications.", selectedChannel))
	return nil
}

func (h *CommandHandler) handleUnsubscribeSubmission(payload InteractionPayload) error {
	ctx := context.Background()
	teamID, channelID := parseMetadata(payload.View.PrivateMetadata)
	if teamID == "" {
		teamID = payload.Team.ID
	}

	if payload.View.State == nil {
		return fmt.Errorf("no view state in payload")
	}

	selectedChannel := ""
	if cv, ok := payload.View.State.Values["channel_block"]["channel_select"]; ok && cv.Selected != nil {
		selectedChannel = cv.Selected.Value
	}
	if selectedChannel == "" {
		return fmt.Errorf("no channel selected")
	}

	log.WithFields(log.Fields{
		"team_id": teamID,
		"channel": selectedChannel,
	}).Debug("unsubscribe modal submission")

	subs, err := h.store.ListSubscriptions(ctx, teamID)
	if err != nil {
		return fmt.Errorf("list subscriptions: %w", err)
	}

	for _, sub := range subs {
		if sub.Target == selectedChannel && sub.Type == "slack" {
			if err := h.store.DeleteSubscription(ctx, sub.ID); err != nil {
				return fmt.Errorf("delete subscription: %w", err)
			}
			log.WithFields(log.Fields{
				"team_id": teamID,
				"channel": selectedChannel,
			}).Info("unsubscribed channel via modal")
			h.postConfirmation(ctx, teamID, channelID, payload.User.ID, fmt.Sprintf("Unsubscribed <#%s> from meeting notifications.", selectedChannel))
			return nil
		}
	}

	return fmt.Errorf("subscription not found for channel %s", selectedChannel)
}

func (h *CommandHandler) handleAdminAddSubmission(payload InteractionPayload) error {
	ctx := context.Background()
	teamID, channelID := parseMetadata(payload.View.PrivateMetadata)
	if teamID == "" {
		teamID = payload.Team.ID
	}

	if payload.View.State == nil {
		return fmt.Errorf("no view state in payload")
	}

	userID := ""
	if uv, ok := payload.View.State.Values["user_block"]["user_select"]; ok && uv.SelectedUser != nil {
		userID = *uv.SelectedUser
	}
	if userID == "" {
		return fmt.Errorf("no user selected")
	}

	if err := h.store.AddAdmin(ctx, teamID, userID); err != nil {
		return fmt.Errorf("add admin: %w", err)
	}

	log.WithFields(log.Fields{
		"team_id":  teamID,
		"admin_id": userID,
	}).Info("added admin via modal")

	// Notify the new admin via DM
	botToken := h.getBotToken(ctx, teamID)
	if botToken != "" {
		api := slackapi.New(botToken)
		msg := fmt.Sprintf("<@%s> added you as a zoom-notifier admin. Run `/zoom-notifier help` to see available commands.", payload.User.ID)
		channel, _, _, err := api.OpenConversationContext(ctx, &slackapi.OpenConversationParameters{Users: []string{userID}})
		if err != nil {
			log.WithError(err).WithField("user_id", userID).Warn("failed to open DM for admin notification")
		} else {
			_, _, err = api.PostMessageContext(ctx, channel.ID, slackapi.MsgOptionText(msg, false))
			if err != nil {
				log.WithError(err).WithField("user_id", userID).Warn("failed to send admin notification DM")
			}
		}
	}

	h.postConfirmation(ctx, teamID, channelID, payload.User.ID, fmt.Sprintf("Added <@%s> as admin.", userID))
	return nil
}

func (h *CommandHandler) handleAddFilterSubmission(payload InteractionPayload) error {
	ctx := context.Background()
	teamID, channelID := parseMetadata(payload.View.PrivateMetadata)
	if teamID == "" {
		teamID = payload.Team.ID
	}

	if payload.View.State == nil {
		return fmt.Errorf("no view state in payload")
	}

	pattern := ""
	if pv, ok := payload.View.State.Values["pattern_block"]["pattern_input"]; ok && pv.Value != nil {
		pattern = *pv.Value
	}
	if pattern == "" {
		return fmt.Errorf("no pattern provided")
	}

	f := &store.MeetingFilter{
		TenantID: teamID,
		Pattern:  pattern,
	}

	// Optional suffix override
	if sv, ok := payload.View.State.Values["suffix_block"]["suffix_input"]; ok && sv.Value != nil && *sv.Value != "" {
		suffix := *sv.Value
		f.MsgSuffix = &suffix
	}

	// Optional link override
	if lv, ok := payload.View.State.Values["link_block"]["link_input"]; ok && lv.Selected != nil {
		switch lv.Selected.Value {
		case "on":
			v := true
			f.IncludeLink = &v
		case "off":
			v := false
			f.IncludeLink = &v
			// "default" means no override
		}
	}

	log.WithFields(log.Fields{
		"team_id": teamID,
		"pattern": pattern,
	}).Debug("add_filter modal submission")

	if err := h.store.CreateFilter(ctx, f); err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"team_id":      teamID,
		"pattern":      pattern,
		"has_suffix":   f.MsgSuffix != nil,
		"has_link_ovr": f.IncludeLink != nil,
	}).Info("created meeting filter via modal")
	h.postConfirmation(ctx, teamID, channelID, payload.User.ID, fmt.Sprintf("Added meeting filter `%s`.", pattern))
	return nil
}

// getFilterBySelectValue parses a filter select value like "filter_123" and returns the filter.
func (h *CommandHandler) getFilterBySelectValue(ctx context.Context, teamID, selectValue string) (*store.MeetingFilter, error) {
	idStr := strings.TrimPrefix(selectValue, "filter_")
	filterID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid filter ID: %s", selectValue)
	}

	filters, err := h.store.ListFilters(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("list filters: %w", err)
	}
	for _, f := range filters {
		if f.ID == filterID {
			return f, nil
		}
	}
	return nil, nil
}
