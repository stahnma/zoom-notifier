package irc

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
	ircevent "github.com/thoj/go-ircevent"
)

// Relay sends messages to IRC channels using per-tenant config.
type Relay struct{}

// Send connects to the IRC server, joins the channel, sends the message, and disconnects.
func (r *Relay) Send(ctx context.Context, config *store.IRCConfig, channel string, msg string) error {
	if config.Server == "" {
		return fmt.Errorf("IRC server is required")
	}
	if config.Nick == "" {
		return fmt.Errorf("IRC nick is required")
	}
	if channel == "" {
		return fmt.Errorf("IRC channel is required")
	}
	if msg == "" {
		return fmt.Errorf("message is required")
	}

	log.WithFields(log.Fields{
		"server":  config.Server,
		"channel": channel,
		"nick":    config.Nick,
		"use_tls": config.UseTLS,
	}).Debug("connecting to IRC server")

	irccon := ircevent.IRC(config.Nick, config.Nick)
	if config.UseTLS {
		irccon.UseTLS = true
		irccon.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}
	irccon.Password = config.Password

	irccon.AddCallback("001", func(e *ircevent.Event) {
		irccon.Join(channel)
	})

	err := irccon.Connect(config.Server)
	if err != nil {
		log.WithError(err).WithField("server", config.Server).Error("failed to connect to IRC server")
		return fmt.Errorf("connect to IRC server %s: %w", config.Server, err)
	}
	defer irccon.Quit()

	irccon.Privmsg(channel, msg)

	log.WithFields(log.Fields{
		"server":  config.Server,
		"channel": channel,
		"nick":    config.Nick,
	}).Info("sent IRC message")

	time.AfterFunc(1*time.Second, func() {
		irccon.Quit()
	})
	irccon.Loop()

	return nil
}
