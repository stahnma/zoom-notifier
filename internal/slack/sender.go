package slack

import (
	"context"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
)

const (
	// slackAPITimeout is the timeout for Slack API requests (sending messages, etc).
	slackAPITimeout = 10 * time.Second
)

// Sender sends notifications to Slack channels using bot tokens.
type Sender struct {
	apiURL     string // empty = use default Slack API URL
	httpClient *http.Client
}

// NewSender creates a Slack sender. Pass empty string for apiURL to use default.
func NewSender(apiURL string) *Sender {
	return &Sender{
		apiURL:     apiURL,
		httpClient: &http.Client{Timeout: slackAPITimeout},
	}
}

// Send posts a message to a Slack channel using the given bot token.
func (s *Sender) Send(ctx context.Context, botToken string, channelID string, msg string) error {
	log.WithFields(log.Fields{
		"channel_id": channelID,
		"msg_length": len(msg),
	}).Debug("sending slack message")

	opts := []slack.Option{
		slack.OptionHTTPClient(s.httpClient),
	}
	if s.apiURL != "" {
		opts = append(opts, slack.OptionAPIURL(s.apiURL))
	}
	api := slack.New(botToken, opts...)

	_, _, err := api.PostMessageContext(ctx, channelID,
		slack.MsgOptionText(msg, false),
		slack.MsgOptionDisableLinkUnfurl(),
		slack.MsgOptionDisableMediaUnfurl(),
	)
	if err != nil {
		log.WithError(err).WithField("channel_id", channelID).Error("failed to send slack message")
		return fmt.Errorf("post to slack channel %s: %w", channelID, err)
	}

	log.WithField("channel_id", channelID).Debug("slack message sent successfully")
	return nil
}
