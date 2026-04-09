package setup

import (
	"bytes"
	"os"
	"strings"
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

// tomlEscape escapes backslashes and double quotes for TOML basic strings.
func tomlEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

var tomlFuncs = template.FuncMap{"toml": tomlEscape}

const configTemplate = `[server]
port = {{.ServerPort}}
host = "{{.ServerHost | toml}}"

[database]
path = "{{.DatabasePath | toml}}"

[zoom]
webhook_secret = "{{.ZoomSecret | toml}}"

[slack]
client_id = "{{.SlackClientID | toml}}"
client_secret = "{{.SlackClientSecret | toml}}"
signing_secret = "{{.SlackSigningSecret | toml}}"

[admin]
api_key = "{{.AdminAPIKey | toml}}"

[log]
level = "{{.LogLevel | toml}}"
`

// WriteConfig renders the TOML config template with the given SetupData and
// writes it to path with 0600 permissions.
func WriteConfig(path string, data *SetupData) error {
	tmpl, err := template.New("config").Funcs(tomlFuncs).Parse(configTemplate)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	return os.WriteFile(path, buf.Bytes(), 0600)
}
