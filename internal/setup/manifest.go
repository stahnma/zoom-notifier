package setup

import (
	"encoding/json"
	"net/url"
)

// slackManifest represents the structure of a Slack app manifest.
type slackManifest struct {
	DisplayInformation displayInformation `json:"display_information"`
	Features           features           `json:"features"`
	OAuthConfig        oauthConfig        `json:"oauth_config"`
	Settings           manifestSettings   `json:"settings"`
}

type displayInformation struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type features struct {
	BotUser       botUser        `json:"bot_user"`
	SlashCommands []slashCommand `json:"slash_commands"`
}

type botUser struct {
	DisplayName  string `json:"display_name"`
	AlwaysOnline bool   `json:"always_online"`
}

type slashCommand struct {
	Command     string `json:"command"`
	URL         string `json:"url"`
	Description string `json:"description"`
	UsageHint   string `json:"usage_hint"`
}

type oauthConfig struct {
	RedirectURLs []string    `json:"redirect_urls"`
	Scopes       oauthScopes `json:"scopes"`
}

type oauthScopes struct {
	Bot []string `json:"bot"`
}

type manifestSettings struct {
	SocketModeEnabled    bool `json:"socket_mode_enabled"`
	OrgDeployEnabled     bool `json:"org_deploy_enabled"`
	TokenRotationEnabled bool `json:"token_rotation_enabled"`
}

// GenerateSlackManifest returns a JSON string containing a Slack app manifest
// configured for the given server URL.
func GenerateSlackManifest(serverURL string) string {
	m := slackManifest{
		DisplayInformation: displayInformation{
			Name:        "zoom-notifier",
			Description: "Zoom meeting notifications for Slack",
		},
		Features: features{
			BotUser: botUser{
				DisplayName:  "zoom-notifier",
				AlwaysOnline: true,
			},
			SlashCommands: []slashCommand{
				{
					Command:     "/zoom-notifier",
					URL:         serverURL + "/slack/commands",
					Description: "Manage Zoom meeting notifications",
					UsageHint:   "[status|subscribe|help]",
				},
			},
		},
		OAuthConfig: oauthConfig{
			RedirectURLs: []string{serverURL + "/slack/callback"},
			Scopes: oauthScopes{
				Bot: []string{"commands", "chat:write", "chat:write.public", "channels:read"},
			},
		},
		Settings: manifestSettings{
			SocketModeEnabled:    false,
			OrgDeployEnabled:     false,
			TokenRotationEnabled: false,
		},
	}

	data, _ := json.MarshalIndent(m, "", "  ")
	return string(data)
}

// SlackManifestURL returns a URL that opens the Slack app creation page
// pre-populated with the generated manifest.
func SlackManifestURL(serverURL string) string {
	manifest := GenerateSlackManifest(serverURL)
	return "https://api.slack.com/apps?new_app=1&manifest_json=" + url.QueryEscape(manifest)
}
