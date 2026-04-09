package setup

import (
	"testing"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/config"
)

func fullConfig() *config.Config {
	return &config.Config{
		Admin: config.AdminConfig{APIKey: "admin-key"},
		Zoom:  config.ZoomConfig{WebhookSecret: "zoom-secret"},
		Slack: config.SlackConfig{
			ClientID:      "client-id",
			ClientSecret:  "client-secret",
			SigningSecret: "signing-secret",
		},
	}
}

func TestNeedsSetup(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{
			name: "nil config",
			cfg:  nil,
			want: true,
		},
		{
			name: "empty config",
			cfg:  &config.Config{},
			want: true,
		},
		{
			name: "missing admin api key",
			cfg: func() *config.Config {
				c := fullConfig()
				c.Admin.APIKey = ""
				return c
			}(),
			want: true,
		},
		{
			name: "missing zoom webhook secret",
			cfg: func() *config.Config {
				c := fullConfig()
				c.Zoom.WebhookSecret = ""
				return c
			}(),
			want: true,
		},
		{
			name: "missing slack client id",
			cfg: func() *config.Config {
				c := fullConfig()
				c.Slack.ClientID = ""
				return c
			}(),
			want: true,
		},
		{
			name: "missing slack client secret",
			cfg: func() *config.Config {
				c := fullConfig()
				c.Slack.ClientSecret = ""
				return c
			}(),
			want: true,
		},
		{
			name: "missing slack signing secret",
			cfg: func() *config.Config {
				c := fullConfig()
				c.Slack.SigningSecret = ""
				return c
			}(),
			want: true,
		},
		{
			name: "all present",
			cfg:  fullConfig(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsSetup(tt.cfg)
			if got != tt.want {
				t.Errorf("NeedsSetup() = %v, want %v", got, tt.want)
			}
		})
	}
}
