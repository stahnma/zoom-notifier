package slack

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

// Bot wraps the Slack Socket Mode client and routes events to CommandHandler.
type Bot struct {
	appToken string
	botToken string
	handler  *CommandHandler
}

func NewBot(appToken, botToken string, handler *CommandHandler) *Bot {
	return &Bot{
		appToken: appToken,
		botToken: botToken,
		handler:  handler,
	}
}

// Start connects to Slack via Socket Mode and listens for slash commands.
// It blocks until the context is cancelled.
func (b *Bot) Start(ctx context.Context) error {
	api := slack.New(b.botToken, slack.OptionAppLevelToken(b.appToken))
	client := socketmode.New(api)

	go func() {
		for evt := range client.Events {
			switch evt.Type {
			case socketmode.EventTypeSlashCommand:
				cmd, ok := evt.Data.(slack.SlashCommand)
				if !ok {
					continue
				}
				b.handleSlashCommand(ctx, client, evt, cmd)

			default:
				log.WithField("type", evt.Type).Debug("ignoring socket mode event")
			}
		}
	}()

	log.Info("starting Slack Socket Mode bot")
	return client.RunContext(ctx)
}

func (b *Bot) handleSlashCommand(ctx context.Context, client *socketmode.Client, evt socketmode.Event, cmd slack.SlashCommand) {
	slashCmd := SlashCommand{
		TeamID:    cmd.TeamID,
		UserID:    cmd.UserID,
		ChannelID: cmd.ChannelID,
		Text:      cmd.Text,
	}

	resp, err := b.handler.Handle(ctx, slashCmd)
	if err != nil {
		log.WithError(err).Error("slash command handler failed")
		client.Ack(*evt.Request, map[string]interface{}{
			"response_type": "ephemeral",
			"text":          fmt.Sprintf("Error: %v", err),
		})
		return
	}

	client.Ack(*evt.Request, map[string]interface{}{
		"response_type": resp.ResponseType,
		"text":          resp.Text,
	})
}
