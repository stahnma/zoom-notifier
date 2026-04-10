package notify

import (
	"context"

	log "github.com/sirupsen/logrus"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/zoom"
)

// SlackSender sends a notification to a Slack channel.
type SlackSender interface {
	Send(ctx context.Context, botToken string, channelID string, msg string) error
}

// IRCSender sends a notification to an IRC channel.
type IRCSender interface {
	Send(ctx context.Context, config *store.IRCConfig, channel string, msg string) error
}

// Dispatcher fans out notifications to matching subscriptions.
type Dispatcher struct {
	store store.Store
	slack SlackSender
	irc   IRCSender
}

func NewDispatcher(s store.Store, slack SlackSender, irc IRCSender) *Dispatcher {
	return &Dispatcher{store: s, slack: slack, irc: irc}
}

// Dispatch finds all matching subscriptions for a webhook event and sends notifications.
func (d *Dispatcher) Dispatch(ctx context.Context, tenantID string, payload zoom.WebhookPayload) {
	topic := payload.Payload.Object.Topic
	meetingID := payload.Payload.Object.ID

	log.WithFields(log.Fields{
		"tenant_id":  tenantID,
		"event":      payload.Event,
		"meeting_id": meetingID,
		"topic":      topic,
	}).Debug("dispatcher received event")

	// Check meeting filters
	matches, err := d.store.MatchesFilter(ctx, tenantID, topic)
	if err != nil {
		log.WithError(err).Error("failed to check meeting filters")
		return
	}
	if !matches {
		log.WithFields(log.Fields{
			"tenant_id": tenantID,
			"topic":     topic,
		}).Debug("meeting filtered out")
		return
	}

	// Get matching subscriptions (enabled, matching meeting_id or wildcard)
	subs, err := d.store.GetSubscriptionsForMeeting(ctx, tenantID, meetingID)
	if err != nil {
		log.WithError(err).Error("failed to get subscriptions")
		return
	}

	log.WithFields(log.Fields{
		"tenant_id":     tenantID,
		"subscriptions": len(subs),
	}).Debug("found matching subscriptions")

	if len(subs) == 0 {
		return
	}

	// Format message
	msg := formatMessage(payload)
	if msg == "" {
		return
	}

	// Get the matching filter for overrides (separate from the pass/fail check above)
	matchedFilter, err := d.store.GetMatchingFilter(ctx, tenantID, topic)
	if err != nil {
		log.WithError(err).Error("failed to get matching filter")
		// non-fatal: fall through to use tenant defaults
	}

	// Get tenant for bot token and defaults
	tenant, err := d.store.GetTenant(ctx, tenantID)
	if err != nil || tenant == nil {
		log.WithError(err).WithField("tenant_id", tenantID).Error("failed to get tenant")
		return
	}

	// Resolve suffix: filter override takes precedence over tenant default
	suffix := tenant.DefaultMsgSuffix
	if matchedFilter != nil && matchedFilter.MsgSuffix != nil {
		suffix = *matchedFilter.MsgSuffix
	}

	// Resolve includeLink: filter override takes precedence over tenant default
	// Not yet used in message formatting but wired up for future use
	includeLink := tenant.DefaultIncludeLink
	if matchedFilter != nil && matchedFilter.IncludeLink != nil {
		includeLink = *matchedFilter.IncludeLink
	}
	// Fetch meeting link if enabled
	var meetingLink string
	if includeLink {
		meetingLink = d.getMeetingLink(ctx, tenantID, meetingID)
	}

	for _, sub := range subs {
		displaySuffix := suffix
		if meetingLink != "" {
			// Slack format: <url|text> makes the suffix a clickable link
			displaySuffix = "<" + meetingLink + "|" + suffix + ">"
		}
		subMsg := formatMessageWithSuffix(payload, displaySuffix)

		log.WithFields(log.Fields{
			"type":    sub.Type,
			"target":  sub.Target,
			"message": subMsg,
		}).Debug("sending notification")

		switch sub.Type {
		case "slack":
			botToken := ""
			if tenant.BotToken != nil {
				botToken = *tenant.BotToken
			}
			if err := d.slack.Send(ctx, botToken, sub.Target, subMsg); err != nil {
				log.WithError(err).WithFields(log.Fields{
					"target":    sub.Target,
					"tenant_id": tenantID,
				}).Error("failed to send slack notification")
			}

		case "irc":
			configs, err := d.store.GetIRCConfig(ctx, tenantID)
			if err != nil || len(configs) == 0 {
				log.WithError(err).WithField("tenant_id", tenantID).Error("no IRC config found")
				continue
			}
			for _, cfg := range configs {
				if err := d.irc.Send(ctx, cfg, sub.Target, subMsg); err != nil {
					log.WithError(err).WithFields(log.Fields{
						"target":    sub.Target,
						"server":    cfg.Server,
						"tenant_id": tenantID,
					}).Error("failed to send IRC notification")
				}
			}
		}
	}
}

// getMeetingLink fetches the join URL for a meeting using the tenant's Zoom API credentials.
func (d *Dispatcher) getMeetingLink(ctx context.Context, tenantID string, meetingID string) string {
	creds, err := d.store.GetZoomCredentials(ctx, tenantID)
	if err != nil || creds == nil {
		log.WithFields(log.Fields{
			"tenant_id": tenantID,
		}).Debug("no zoom API credentials configured, skipping meeting link")
		return ""
	}

	client := zoom.NewAPIClient("https://zoom.us", "https://api.zoom.us", creds.ClientID, creds.ClientSecret, creds.AccountID)
	link, err := client.GetMeetingJoinLink(meetingID)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"tenant_id":  tenantID,
			"meeting_id": meetingID,
		}).Warn("failed to fetch meeting join link")
		return ""
	}

	log.WithFields(log.Fields{
		"tenant_id":  tenantID,
		"meeting_id": meetingID,
	}).Debug("fetched meeting join link")
	return link
}

func formatMessage(payload zoom.WebhookPayload) string {
	userName := payload.Payload.Object.Participant.UserName
	switch payload.Event {
	case "meeting.participant_joined":
		return userName + " has joined "
	case "meeting.participant_left":
		return userName + " has left "
	default:
		return ""
	}
}

func formatMessageWithSuffix(payload zoom.WebhookPayload, suffix string) string {
	base := formatMessage(payload)
	if base == "" {
		return ""
	}
	return base + suffix
}
