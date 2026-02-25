package slack

import (
	"context"
	"fmt"

	"github.com/slack-go/slack"
)

// Sender sends notifications to Slack channels using bot tokens.
type Sender struct {
	apiURL string // empty = use default Slack API URL
}

// NewSender creates a Slack sender. Pass empty string for apiURL to use default.
func NewSender(apiURL string) *Sender {
	return &Sender{apiURL: apiURL}
}

// Send posts a message to a Slack channel using the given bot token.
func (s *Sender) Send(ctx context.Context, botToken string, channelID string, msg string) error {
	opts := []slack.Option{}
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
		return fmt.Errorf("post to slack channel %s: %w", channelID, err)
	}
	return nil
}
