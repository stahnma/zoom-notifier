package slack

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

// RegisterModalHandlers registers all modal submission handlers on the InteractionHandler.
func (h *CommandHandler) RegisterModalHandlers(ih *InteractionHandler) {
	ih.RegisterHandler("set_suffix", h.handleSetSuffixSubmission)
	ih.RegisterHandler("set_link", h.handleSetLinkSubmission)
	ih.RegisterHandler("subscribe", h.handleSubscribeSubmission)
	ih.RegisterHandler("add_filter", h.handleAddFilterSubmission)
}

func (h *CommandHandler) handleSetSuffixSubmission(payload InteractionPayload) error {
	ctx := context.Background()
	teamID := payload.View.PrivateMetadata
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
		// Tenant-wide default
		tenant, err := h.store.GetTenant(ctx, teamID)
		if err != nil || tenant == nil {
			return fmt.Errorf("get tenant: %w", err)
		}
		return h.store.UpdateTenantDefaults(ctx, teamID, suffixValue, tenant.DefaultIncludeLink)
	}

	// Filter override: filterValue is "filter_<id>"
	filter, err := h.getFilterBySelectValue(ctx, teamID, filterValue)
	if err != nil {
		return err
	}
	if filter == nil {
		return fmt.Errorf("filter not found: %s", filterValue)
	}
	filter.MsgSuffix = &suffixValue
	return h.store.UpdateFilter(ctx, filter)
}

func (h *CommandHandler) handleSetLinkSubmission(payload InteractionPayload) error {
	ctx := context.Background()
	teamID := payload.View.PrivateMetadata
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

	if filterValue == "" || filterValue == "all" {
		// Tenant-wide default
		tenant, err := h.store.GetTenant(ctx, teamID)
		if err != nil || tenant == nil {
			return fmt.Errorf("get tenant: %w", err)
		}
		return h.store.UpdateTenantDefaults(ctx, teamID, tenant.DefaultMsgSuffix, includeLink)
	}

	// Filter override
	filter, err := h.getFilterBySelectValue(ctx, teamID, filterValue)
	if err != nil {
		return err
	}
	if filter == nil {
		return fmt.Errorf("filter not found: %s", filterValue)
	}
	filter.IncludeLink = &includeLink
	return h.store.UpdateFilter(ctx, filter)
}

func (h *CommandHandler) handleSubscribeSubmission(payload InteractionPayload) error {
	ctx := context.Background()
	teamID := payload.View.PrivateMetadata
	if teamID == "" {
		teamID = payload.Team.ID
	}

	if payload.View.State == nil {
		return fmt.Errorf("no view state in payload")
	}

	channelID := ""
	if cv, ok := payload.View.State.Values["channel_block"]["channel_select"]; ok {
		if cv.SelectedConversation != nil {
			channelID = *cv.SelectedConversation
		}
	}
	if channelID == "" {
		return fmt.Errorf("no channel selected")
	}

	log.WithFields(log.Fields{
		"team_id": teamID,
		"channel": channelID,
	}).Debug("subscribe modal submission")

	// Check for duplicate
	existing, err := h.store.ListSubscriptions(ctx, teamID)
	if err != nil {
		return fmt.Errorf("list subscriptions: %w", err)
	}
	for _, s := range existing {
		if s.Target == channelID && s.Type == "slack" {
			return nil // already subscribed, silently succeed
		}
	}

	sub := &store.Subscription{
		TenantID: teamID,
		Type:     "slack",
		Target:   channelID,
		Enabled:  true,
	}
	return h.store.CreateSubscription(ctx, sub)
}

func (h *CommandHandler) handleAddFilterSubmission(payload InteractionPayload) error {
	ctx := context.Background()
	teamID := payload.View.PrivateMetadata
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

	return h.store.CreateFilter(ctx, f)
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
