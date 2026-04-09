package setup

import (
	"bytes"
	"os"
	"text/template"
)

// SetupData holds the values collected by the setup wizard for writing a config file.
type SetupData struct {
	ServerURL          string
	AdminAPIKey        string
	ZoomSecret         string
	SlackClientID      string
	SlackClientSecret  string
	SlackSigningSecret string
	ServerHost         string
	ServerPort         int
	DatabasePath       string
	LogLevel           string
}

const configTemplate = `[server]
port = {{.ServerPort}}
host = "{{.ServerHost}}"

[database]
path = "{{.DatabasePath}}"

[zoom]
webhook_secret = "{{.ZoomSecret}}"

[slack]
client_id = "{{.SlackClientID}}"
client_secret = "{{.SlackClientSecret}}"
signing_secret = "{{.SlackSigningSecret}}"

[admin]
api_key = "{{.AdminAPIKey}}"

[log]
level = "{{.LogLevel}}"
`

// WriteConfig renders the TOML config template with the given SetupData and
// writes it to path with 0600 permissions.
func WriteConfig(path string, data *SetupData) error {
	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	return os.WriteFile(path, buf.Bytes(), 0600)
}
