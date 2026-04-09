package setup

import (
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/config"
)

// NeedsSetup returns true if the configuration is missing any required fields
// and the interactive setup wizard should be launched.
func NeedsSetup(cfg *config.Config) bool {
	if cfg == nil {
		return true
	}
	if cfg.Admin.APIKey == "" {
		return true
	}
	if cfg.Zoom.WebhookSecret == "" {
		return true
	}
	if cfg.Slack.ClientID == "" {
		return true
	}
	if cfg.Slack.ClientSecret == "" {
		return true
	}
	if cfg.Slack.SigningSecret == "" {
		return true
	}
	return false
}
