package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Zoom     ZoomConfig
	Slack    SlackConfig
	Admin    AdminConfig
	Log      LogConfig
}

type ServerConfig struct {
	Port int
	Host string
	URL  string
}

type DatabaseConfig struct {
	Path          string
	EncryptionKey string
}

type ZoomConfig struct {
	WebhookSecret string
	AccountID     string
	ClientID      string
	ClientSecret  string
}

type SlackConfig struct {
	ClientID      string
	ClientSecret  string
	SigningSecret string
}

type AdminConfig struct {
	APIKey string
}

type LogConfig struct {
	Level string
}

func Load(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("toml")

	// Defaults
	v.SetDefault("server.port", 8888)
	v.SetDefault("server.host", "localhost")
	v.SetDefault("database.path", "./zoom-notifier.db")
	v.SetDefault("log.level", "info")

	// Environment variable bindings
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.BindEnv("zoom.webhook_secret", "ZOOM_SECRET")
	v.BindEnv("database.encryption_key", "ZOOMNOTIFIER_DB_KEY")
	v.BindEnv("slack.client_id", "SLACK_CLIENT_ID")
	v.BindEnv("slack.client_secret", "SLACK_CLIENT_SECRET")
	v.BindEnv("slack.signing_secret", "SLACK_SIGNING_SECRET")
	v.BindEnv("admin.api_key", "ZOOMNOTIFIER_ADMIN_KEY")

	// Load config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.config/zoom-notifier")
		v.AddConfigPath("/etc/zoom-notifier")
	}

	// Read config file (ignore "not found" errors — env/defaults are fine)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && configPath != "" {
			return nil, err
		}
	}

	cfg := &Config{}
	cfg.Server.Port = v.GetInt("server.port")
	cfg.Server.Host = v.GetString("server.host")
	cfg.Server.URL = v.GetString("server.url")
	cfg.Database.Path = v.GetString("database.path")
	cfg.Database.EncryptionKey = v.GetString("database.encryption_key")
	cfg.Zoom.WebhookSecret = v.GetString("zoom.webhook_secret")
	cfg.Zoom.AccountID = v.GetString("zoom.account_id")
	cfg.Zoom.ClientID = v.GetString("zoom.client_id")
	cfg.Zoom.ClientSecret = v.GetString("zoom.client_secret")
	cfg.Slack.ClientID = v.GetString("slack.client_id")
	cfg.Slack.ClientSecret = v.GetString("slack.client_secret")
	cfg.Slack.SigningSecret = v.GetString("slack.signing_secret")
	cfg.Admin.APIKey = v.GetString("admin.api_key")
	cfg.Log.Level = v.GetString("log.level")

	return cfg, nil
}
