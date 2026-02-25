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

	// Format message
	msg := formatMessage(payload)
	if msg == "" {
		return
	}

	// Get tenant for bot token
	tenant, err := d.store.GetTenant(ctx, tenantID)
	if err != nil || tenant == nil {
		log.WithError(err).WithField("tenant_id", tenantID).Error("failed to get tenant")
		return
	}

	for _, sub := range subs {
		subMsg := formatMessageWithSuffix(payload, sub.MsgSuffix)

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
