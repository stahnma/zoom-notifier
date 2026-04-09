# Setup Wizard Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a web-based setup wizard that auto-launches when no configuration exists, walking users through Zoom and Slack credential setup and writing a TOML config file.

**Architecture:** New `internal/setup/` package with embedded HTML templates, a multi-step form wizard, and config file writer. Integrated into `main.go` with early config check. Also replaces Socket Mode slash commands with HTTP-based handler verified by Slack signing secret.

**Tech Stack:** Go `embed`, `html/template`, `net/http`, `crypto/hmac` for Slack signature verification, Chi router for setup routes, existing Viper for config validation.

---

### Task 1: Setup Mode Detection

Create the function that determines whether setup is needed.

**Files:**
- Create: `internal/setup/setup.go`
- Test: `internal/setup/setup_test.go`

**Step 1: Write the failing test**

```go
// internal/setup/setup_test.go
package setup

import (
	"testing"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/config"
)

func TestNeedsSetup(t *testing.T) {
	tests := []struct {
		name   string
		cfg    *config.Config
		want   bool
	}{
		{
			name: "nil config needs setup",
			cfg:  nil,
			want: true,
		},
		{
			name: "empty config needs setup",
			cfg:  &config.Config{},
			want: true,
		},
		{
			name: "missing slack client ID needs setup",
			cfg: &config.Config{
				Admin: config.AdminConfig{APIKey: "key"},
				Zoom:  config.ZoomConfig{WebhookSecret: "secret"},
			},
			want: true,
		},
		{
			name: "missing zoom secret needs setup",
			cfg: &config.Config{
				Admin: config.AdminConfig{APIKey: "key"},
				Slack: config.SlackConfig{ClientID: "id", ClientSecret: "sec", SigningSecret: "sig"},
			},
			want: true,
		},
		{
			name: "missing admin key needs setup",
			cfg: &config.Config{
				Zoom:  config.ZoomConfig{WebhookSecret: "secret"},
				Slack: config.SlackConfig{ClientID: "id", ClientSecret: "sec", SigningSecret: "sig"},
			},
			want: true,
		},
		{
			name: "all required fields present",
			cfg: &config.Config{
				Admin: config.AdminConfig{APIKey: "key"},
				Zoom:  config.ZoomConfig{WebhookSecret: "secret"},
				Slack: config.SlackConfig{ClientID: "id", ClientSecret: "sec", SigningSecret: "sig"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsSetup(tt.cfg); got != tt.want {
				t.Errorf("NeedsSetup() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/setup/ -v -run TestNeedsSetup`
Expected: FAIL — package doesn't exist yet.

**Step 3: Write minimal implementation**

```go
// internal/setup/setup.go
package setup

import "github.com/stahnma/mandatoryFun/zoom-notifier/internal/config"

// NeedsSetup returns true if the configuration is missing required values.
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
	if cfg.Slack.ClientID == "" || cfg.Slack.ClientSecret == "" || cfg.Slack.SigningSecret == "" {
		return true
	}
	return false
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/setup/ -v -run TestNeedsSetup`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/setup/setup.go internal/setup/setup_test.go
git commit -m "feat(setup): add NeedsSetup config detection"
```

---

### Task 2: API Key Generation Utility

Add a helper to generate cryptographically secure API keys (reusable from the setup wizard). The `generateOAuthAPIKey` function in `internal/slack/oauth.go:170-176` already does this but is unexported and Slack-specific.

**Files:**
- Create: `internal/setup/keygen.go`
- Test: `internal/setup/keygen_test.go`

**Step 1: Write the failing test**

```go
// internal/setup/keygen_test.go
package setup

import (
	"testing"
)

func TestGenerateAPIKey(t *testing.T) {
	key, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error: %v", err)
	}
	if len(key) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("expected 64 char hex string, got %d chars", len(key))
	}

	// Keys should be unique
	key2, _ := GenerateAPIKey()
	if key == key2 {
		t.Error("two generated keys should not be identical")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/setup/ -v -run TestGenerateAPIKey`
Expected: FAIL — `GenerateAPIKey` not defined.

**Step 3: Write minimal implementation**

```go
// internal/setup/keygen.go
package setup

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateAPIKey returns a 64-character hex string from 32 random bytes.
func GenerateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return hex.EncodeToString(b), nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/setup/ -v -run TestGenerateAPIKey`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/setup/keygen.go internal/setup/keygen_test.go
git commit -m "feat(setup): add API key generation utility"
```

---

### Task 3: Config File Writer

Write a function that takes the wizard form data and writes a TOML config file with `0600` permissions.

**Files:**
- Create: `internal/setup/configwriter.go`
- Test: `internal/setup/configwriter_test.go`

**Step 1: Write the failing test**

```go
// internal/setup/configwriter_test.go
package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	data := &SetupData{
		ServerURL:     "https://zoom.example.com",
		AdminAPIKey:   "testkey123",
		ZoomSecret:    "zoomsecret456",
		SlackClientID:     "slack-client-id",
		SlackClientSecret: "slack-client-secret",
		SlackSigningSecret: "slack-signing-secret",
		ServerHost:    "localhost",
		ServerPort:    8888,
		DatabasePath:  "./zoom-notifier.db",
		LogLevel:      "info",
	}

	if err := WriteConfig(path, data); err != nil {
		t.Fatalf("WriteConfig() error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	// Verify key values are present
	s := string(content)
	for _, want := range []string{
		`port = 8888`,
		`host = "localhost"`,
		`path = "./zoom-notifier.db"`,
		`webhook_secret = "zoomsecret456"`,
		`client_id = "slack-client-id"`,
		`client_secret = "slack-client-secret"`,
		`signing_secret = "slack-signing-secret"`,
		`api_key = "testkey123"`,
		`level = "info"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("config missing %q", want)
		}
	}

	// Verify file permissions
	info, _ := os.Stat(path)
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected 0600 permissions, got %o", perm)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/setup/ -v -run TestWriteConfig`
Expected: FAIL — `SetupData` and `WriteConfig` not defined.

**Step 3: Write minimal implementation**

```go
// internal/setup/configwriter.go
package setup

import (
	"fmt"
	"os"
	"text/template"
)

// SetupData holds all values collected by the wizard.
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

// WriteConfig writes a TOML config file from the setup wizard data.
func WriteConfig(path string, data *SetupData) error {
	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return fmt.Errorf("parse config template: %w", err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/setup/ -v -run TestWriteConfig`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/setup/configwriter.go internal/setup/configwriter_test.go
git commit -m "feat(setup): add TOML config file writer"
```

---

### Task 4: Slack Manifest Generator

Build the Slack app manifest JSON and the URL that opens "Create App from Manifest" on Slack.

**Files:**
- Create: `internal/setup/manifest.go`
- Test: `internal/setup/manifest_test.go`

**Step 1: Write the failing test**

```go
// internal/setup/manifest_test.go
package setup

import (
	"encoding/json"
	"testing"
)

func TestGenerateSlackManifest(t *testing.T) {
	manifest := GenerateSlackManifest("https://zoom.example.com")

	// Should be valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(manifest), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Check essential fields exist
	if _, ok := parsed["display_information"]; !ok {
		t.Error("missing display_information")
	}
	if _, ok := parsed["features"]; !ok {
		t.Error("missing features")
	}
	if _, ok := parsed["oauth_config"]; !ok {
		t.Error("missing oauth_config")
	}
}

func TestSlackManifestURL(t *testing.T) {
	url := SlackManifestURL("https://zoom.example.com")
	if url == "" {
		t.Fatal("empty URL")
	}
	if len(url) < 50 {
		t.Error("URL suspiciously short")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/setup/ -v -run TestGenerateSlackManifest`
Expected: FAIL — `GenerateSlackManifest` not defined.

**Step 3: Write minimal implementation**

```go
// internal/setup/manifest.go
package setup

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// GenerateSlackManifest returns the Slack app manifest JSON for the given server URL.
func GenerateSlackManifest(serverURL string) string {
	manifest := map[string]interface{}{
		"display_information": map[string]interface{}{
			"name":        "zoom-notifier",
			"description": "Zoom meeting notifications for Slack",
		},
		"features": map[string]interface{}{
			"bot_user": map[string]interface{}{
				"display_name":  "zoom-notifier",
				"always_online": true,
			},
			"slash_commands": []map[string]interface{}{
				{
					"command":      "/zoom-notifier",
					"url":          serverURL + "/slack/commands",
					"description":  "Manage Zoom meeting notifications",
					"usage_hint":   "[status|subscribe|help]",
					"should_escape": false,
				},
			},
		},
		"oauth_config": map[string]interface{}{
			"redirect_urls": []string{
				serverURL + "/slack/callback",
			},
			"scopes": map[string]interface{}{
				"bot": []string{"commands", "chat:write", "channels:read"},
			},
		},
		"settings": map[string]interface{}{
			"org_deploy_enabled":     false,
			"socket_mode_enabled":    false,
			"token_rotation_enabled": false,
		},
	}

	b, _ := json.Marshal(manifest)
	return string(b)
}

// SlackManifestURL returns the Slack "create app from manifest" URL.
func SlackManifestURL(serverURL string) string {
	manifest := GenerateSlackManifest(serverURL)
	return fmt.Sprintf("https://api.slack.com/apps?new_app=1&manifest_json=%s", url.QueryEscape(manifest))
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/setup/ -v -run "TestGenerateSlackManifest|TestSlackManifestURL"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/setup/manifest.go internal/setup/manifest_test.go
git commit -m "feat(setup): add Slack app manifest generator"
```

---

### Task 5: Setup Wizard HTML Templates & Static Assets

Create the embedded HTML templates for all wizard steps and minimal CSS/JS.

**Files:**
- Create: `internal/setup/templates/layout.html`
- Create: `internal/setup/templates/welcome.html`
- Create: `internal/setup/templates/server-url.html`
- Create: `internal/setup/templates/admin-key.html`
- Create: `internal/setup/templates/zoom.html`
- Create: `internal/setup/templates/slack.html`
- Create: `internal/setup/templates/advanced.html`
- Create: `internal/setup/templates/review.html`
- Create: `internal/setup/templates/complete.html`
- Create: `internal/setup/static/style.css`
- Create: `internal/setup/static/setup.js`

**Step 1: Create the base layout template**

`layout.html` provides the HTML shell with progress indicator, CSS include, and a `{{block "content" .}}` placeholder.

The progress bar shows steps: Welcome → Server URL → Admin Key → Zoom → Slack → Advanced → Review.

**Step 2: Create each step template**

Each template extends layout and defines the `"content"` block with:
- A heading and description
- Form fields with labels
- Collapsible help sections (use `<details><summary>` for zero-JS progressive enhancement)
- "Back" and "Next" buttons that POST to `/setup/step/{n}`

Key details per step:

- **welcome.html**: Checklist of prerequisites, "Get Started" button.
- **server-url.html**: Single text input for the public URL. Help text explains when to use a real domain vs localhost.
- **admin-key.html**: Pre-filled input with generated key. "Regenerate" link (JS). Help text explains what admin API key controls.
- **zoom.html**: Webhook secret input. Displays the webhook endpoint URL (`{serverURL}/webhook/zoom`) with copy button. Help toggle with Zoom App Marketplace step-by-step.
- **slack.html**: "Create Slack App" button linking to manifest URL (generated from server URL). Three input fields: client ID, client secret, signing secret. Each with help toggle showing where to find the value.
- **advanced.html**: Expandable section with host, port, db path, log level fields. All pre-filled with defaults.
- **review.html**: Read-only summary of all values (secrets masked with "show" toggle). Expandable env var export section. "Save Configuration" button.
- **complete.html**: Success message with restart instructions.

**Step 3: Create CSS**

`style.css` — clean, minimal stylesheet. System font stack, max-width container, form styling, progress indicator, `<details>` styling for help toggles, copy button styling. No framework needed. Target ~100 lines.

**Step 4: Create JS**

`setup.js` — minimal vanilla JavaScript for:
- Copy-to-clipboard on endpoint URLs
- "Regenerate" API key button (fetch from `/setup/api/generate-key`)
- "Show/hide" toggle on masked secrets in review
- No frameworks. Target ~50 lines.

**Step 5: Commit**

```bash
git add internal/setup/templates/ internal/setup/static/
git commit -m "feat(setup): add wizard HTML templates and static assets"
```

---

### Task 6: Setup HTTP Handlers

Wire up the setup wizard as an HTTP server with session state.

**Files:**
- Create: `internal/setup/handler.go`
- Test: `internal/setup/handler_test.go`

**Step 1: Write the failing test**

```go
// internal/setup/handler_test.go
package setup

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetupHandlerRedirectsToWelcome(t *testing.T) {
	h := NewHandler("./test-config.toml")
	srv := httptest.NewServer(h.Router())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/setup")
	if err != nil {
		t.Fatalf("GET /setup: %v", err)
	}
	// Should serve the welcome page (200) or redirect to /setup/welcome
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		t.Errorf("expected 200 or 302, got %d", resp.StatusCode)
	}
}

func TestSetupStaticAssets(t *testing.T) {
	h := NewHandler("./test-config.toml")
	srv := httptest.NewServer(h.Router())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/setup/static/style.css")
	if err != nil {
		t.Fatalf("GET style.css: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestSetupGenerateKeyEndpoint(t *testing.T) {
	h := NewHandler("./test-config.toml")
	srv := httptest.NewServer(h.Router())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/setup/api/generate-key")
	if err != nil {
		t.Fatalf("GET generate-key: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/setup/ -v -run TestSetupHandler`
Expected: FAIL — `NewHandler` not defined.

**Step 3: Write the implementation**

```go
// internal/setup/handler.go
package setup

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	log "github.com/sirupsen/logrus"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Handler serves the setup wizard UI.
type Handler struct {
	configPath string
	templates  *template.Template
	data       *SetupData // accumulated across steps
}

func NewHandler(configPath string) *Handler {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))
	apiKey, _ := GenerateAPIKey()
	return &Handler{
		configPath: configPath,
		templates:  tmpl,
		data: &SetupData{
			AdminAPIKey:  apiKey,
			ServerHost:   "localhost",
			ServerPort:   8888,
			DatabasePath: "./zoom-notifier.db",
			LogLevel:     "info",
		},
	}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()

	// Static assets
	staticSub, _ := fs.Sub(staticFS, "static")
	r.Handle("/setup/static/*", http.StripPrefix("/setup/static/", http.FileServer(http.FS(staticSub))))

	// Wizard step pages
	r.Get("/setup", h.welcome)
	r.Get("/setup/welcome", h.welcome)
	r.Post("/setup/server-url", h.handleServerURL)
	r.Post("/setup/admin-key", h.handleAdminKey)
	r.Post("/setup/zoom", h.handleZoom)
	r.Post("/setup/slack", h.handleSlack)
	r.Post("/setup/advanced", h.handleAdvanced)
	r.Post("/setup/save", h.handleSave)

	// API endpoints for JS
	r.Get("/setup/api/generate-key", h.generateKey)
	r.Get("/setup/api/manifest-url", h.manifestURL)

	return r
}

// Each handler:
// 1. Parses form data from the POST (saves to h.data)
// 2. Renders the next step's template

func (h *Handler) welcome(w http.ResponseWriter, r *http.Request) {
	h.render(w, "welcome.html", nil)
}

func (h *Handler) handleServerURL(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	h.data.ServerURL = r.FormValue("server_url")
	h.render(w, "admin-key.html", h.data)
}

func (h *Handler) handleAdminKey(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	h.data.AdminAPIKey = r.FormValue("admin_api_key")
	h.render(w, "zoom.html", h.data)
}

func (h *Handler) handleZoom(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	h.data.ZoomSecret = r.FormValue("zoom_secret")
	manifestURL := SlackManifestURL(h.data.ServerURL)
	h.render(w, "slack.html", map[string]interface{}{
		"Data":        h.data,
		"ManifestURL": manifestURL,
	})
}

func (h *Handler) handleSlack(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	h.data.SlackClientID = r.FormValue("slack_client_id")
	h.data.SlackClientSecret = r.FormValue("slack_client_secret")
	h.data.SlackSigningSecret = r.FormValue("slack_signing_secret")
	h.render(w, "advanced.html", h.data)
}

func (h *Handler) handleAdvanced(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	h.data.ServerHost = r.FormValue("server_host")
	if port, err := strconv.Atoi(r.FormValue("server_port")); err == nil {
		h.data.ServerPort = port
	}
	h.data.DatabasePath = r.FormValue("database_path")
	h.data.LogLevel = r.FormValue("log_level")
	h.render(w, "review.html", h.data)
}

func (h *Handler) handleSave(w http.ResponseWriter, r *http.Request) {
	if err := WriteConfig(h.configPath, h.data); err != nil {
		log.WithError(err).Error("failed to write config")
		http.Error(w, "Failed to write config file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.render(w, "complete.html", h.data)
}

func (h *Handler) generateKey(w http.ResponseWriter, r *http.Request) {
	key, err := GenerateAPIKey()
	if err != nil {
		http.Error(w, "failed to generate key", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"key": key})
}

func (h *Handler) manifestURL(w http.ResponseWriter, r *http.Request) {
	serverURL := r.URL.Query().Get("server_url")
	if serverURL == "" {
		serverURL = h.data.ServerURL
	}
	url := SlackManifestURL(serverURL)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": url})
}

func (h *Handler) render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, name, data); err != nil {
		log.WithError(err).WithField("template", name).Error("failed to render template")
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/setup/ -v -run "TestSetupHandler|TestSetupStatic|TestSetupGenerate"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/setup/handler.go internal/setup/handler_test.go
git commit -m "feat(setup): add wizard HTTP handlers with embedded templates"
```

---

### Task 7: Integrate Setup Mode into main.go

Add the setup mode check to the application entry point.

**Files:**
- Modify: `cmd/zoom-notifier/main.go`

**Step 1: Add `--setup-listen` flag and setup mode check**

After the existing flag definitions (line 33-35 area), add:

```go
setupListen := flag.String("setup-listen", "localhost:8888", "Listen address for setup wizard (only used when config is missing)")
```

After config loading (line 43-46 area), before logging setup, add:

```go
if setup.NeedsSetup(cfg) {
	log.Infof("No configuration found. Setup wizard available at http://%s/setup", *setupListen)
	setupHandler := setup.NewHandler(configPathResolved())
	srv := &http.Server{
		Addr:    *setupListen,
		Handler: setupHandler.Router(),
	}
	// Graceful shutdown for setup mode too
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		srv.Shutdown(context.Background())
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("setup server error: %v", err)
	}
	return
}
```

The `configPathResolved()` helper determines the output path: use `--config` value if set, otherwise `./config.toml`.

**Step 2: Add import for setup package**

Add to imports:

```go
"github.com/stahnma/mandatoryFun/zoom-notifier/internal/setup"
```

**Step 3: Build and test manually**

Run: `make build`
Expected: Compiles successfully.

Run: `./bin/zoom-notifier` (with no config file present)
Expected: Logs "No configuration found. Setup wizard available at http://localhost:8888/setup"

**Step 4: Commit**

```bash
git add cmd/zoom-notifier/main.go
git commit -m "feat: integrate setup wizard into main startup"
```

---

### Task 8: HTTP Slash Command Handler (Replace Socket Mode)

Replace the Socket Mode bot with an HTTP endpoint that receives slash commands via POST and verifies them using the Slack signing secret.

**Files:**
- Create: `internal/slack/http_commands.go`
- Create: `internal/slack/http_commands_test.go`
- Modify: `cmd/zoom-notifier/main.go` (remove bot startup, add route)

**Step 1: Write the failing test**

```go
// internal/slack/http_commands_test.go
package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func signRequest(t *testing.T, body string, secret string, ts string) http.Header {
	t.Helper()
	baseString := fmt.Sprintf("v0:%s:%s", ts, body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(baseString))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))
	h := http.Header{}
	h.Set("X-Slack-Request-Timestamp", ts)
	h.Set("X-Slack-Signature", sig)
	h.Set("Content-Type", "application/x-www-form-urlencoded")
	return h
}

func TestHTTPCommandHandler_ValidRequest(t *testing.T) {
	store := newMockStore() // reuse existing mock from commands_test.go
	cmdHandler := NewCommandHandler(store)
	h := NewHTTPCommandHandler(cmdHandler, "test-signing-secret")

	body := "team_id=T123&user_id=U456&channel_id=C789&text=help&command=%2Fzoom-notifier"
	ts := fmt.Sprintf("%d", time.Now().Unix())
	headers := signRequest(t, body, "test-signing-secret", ts)

	req := httptest.NewRequest("POST", "/slack/commands", strings.NewReader(body))
	req.Header = headers
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPCommandHandler_InvalidSignature(t *testing.T) {
	store := newMockStore()
	cmdHandler := NewCommandHandler(store)
	h := NewHTTPCommandHandler(cmdHandler, "test-signing-secret")

	body := "team_id=T123&user_id=U456&channel_id=C789&text=help"
	req := httptest.NewRequest("POST", "/slack/commands", strings.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set("X-Slack-Signature", "v0=invalidsignature")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHTTPCommandHandler_ExpiredTimestamp(t *testing.T) {
	store := newMockStore()
	cmdHandler := NewCommandHandler(store)
	h := NewHTTPCommandHandler(cmdHandler, "test-signing-secret")

	body := "team_id=T123&user_id=U456&channel_id=C789&text=help"
	oldTs := fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix())
	headers := signRequest(t, body, "test-signing-secret", oldTs)

	req := httptest.NewRequest("POST", "/slack/commands", strings.NewReader(body))
	req.Header = headers
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/slack/ -v -run TestHTTPCommandHandler`
Expected: FAIL — `NewHTTPCommandHandler` not defined.

**Step 3: Write minimal implementation**

```go
// internal/slack/http_commands.go
package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
)

const maxTimestampAge = 5 * time.Minute

// HTTPCommandHandler receives Slack slash commands via HTTP POST.
type HTTPCommandHandler struct {
	handler       *CommandHandler
	signingSecret string
}

func NewHTTPCommandHandler(handler *CommandHandler, signingSecret string) *HTTPCommandHandler {
	return &HTTPCommandHandler{
		handler:       handler,
		signingSecret: signingSecret,
	}
}

func (h *HTTPCommandHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	if !h.verifySignature(r.Header, body) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	values, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	cmd := SlashCommand{
		TeamID:    values.Get("team_id"),
		UserID:    values.Get("user_id"),
		ChannelID: values.Get("channel_id"),
		Text:      values.Get("text"),
	}

	resp, err := h.handler.Handle(r.Context(), cmd)
	if err != nil {
		log.WithError(err).Error("slash command handler failed")
		http.Error(w, fmt.Sprintf("Error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"response_type": resp.ResponseType,
		"text":          resp.Text,
	})
}

func (h *HTTPCommandHandler) verifySignature(header http.Header, body []byte) bool {
	ts := header.Get("X-Slack-Request-Timestamp")
	sig := header.Get("X-Slack-Signature")

	if ts == "" || sig == "" {
		return false
	}

	tsInt, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}

	age := time.Since(time.Unix(tsInt, 0))
	if math.Abs(age.Seconds()) > maxTimestampAge.Seconds() {
		return false
	}

	baseString := fmt.Sprintf("v0:%s:%s", ts, string(body))
	mac := hmac.New(sha256.New, []byte(h.signingSecret))
	mac.Write([]byte(baseString))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sig), []byte(expected))
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/slack/ -v -run TestHTTPCommandHandler`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/slack/http_commands.go internal/slack/http_commands_test.go
git commit -m "feat(slack): add HTTP slash command handler with signing secret verification"
```

---

### Task 9: Wire HTTP Slash Commands into main.go and Remove Socket Mode

Replace the Socket Mode bot startup with the HTTP command route.

**Files:**
- Modify: `cmd/zoom-notifier/main.go`

**Step 1: Remove Socket Mode bot startup**

Remove lines 112-120 (the `if cfg.Slack.AppToken != ""` block) from `main.go`.

**Step 2: Add HTTP slash command route**

After the Slack OAuth routes block (around line 103), add:

```go
if cfg.Slack.SigningSecret != "" {
	cmdHandler := appslack.NewCommandHandler(store)
	httpCmdHandler := appslack.NewHTTPCommandHandler(cmdHandler, cfg.Slack.SigningSecret)
	r.Post("/slack/commands", httpCmdHandler.ServeHTTP)
	log.Info("Slack slash command HTTP endpoint enabled")
}
```

**Step 3: Remove the app_token config references**

In `internal/config/config.go`, remove the `AppToken` field from `SlackConfig` (line 33) and the `BindEnv` for `slack.app_token` (line 63).

**Step 4: Build and verify**

Run: `make build`
Expected: Compiles. May get warnings about unused imports if `socketmode` was imported elsewhere — check and clean up.

**Step 5: Run all tests**

Run: `go test ./internal/... -v`
Expected: All pass. If any bot-related tests exist, they will need updating or removal.

**Step 6: Commit**

```bash
git add cmd/zoom-notifier/main.go internal/config/config.go
git commit -m "feat: replace Socket Mode with HTTP slash commands in main startup"
```

---

### Task 10: Remove Socket Mode Code and Dependency

Clean up the now-unused Socket Mode bot.

**Files:**
- Delete: `internal/slack/bot.go`
- Modify: `go.mod` (remove `github.com/slack-go/slack` if no longer needed — but it IS still needed by `sender.go`)

**Step 1: Delete bot.go**

```bash
rm internal/slack/bot.go
```

**Step 2: Check for remaining Socket Mode imports**

Run: `grep -r "socketmode" internal/`
Expected: No results.

Run: `grep -r "slack.OptionAppLevelToken" internal/`
Expected: No results.

**Step 3: Tidy dependencies**

```bash
go mod tidy
```

This will remove `github.com/gorilla/websocket` (indirect dep of socketmode) if nothing else needs it.

**Step 4: Build and test**

Run: `make build && go test ./internal/... -v`
Expected: All pass, clean build.

**Step 5: Commit**

```bash
git add -A
git commit -m "refactor(slack): remove Socket Mode bot code and tidy dependencies"
```

---

### Task 11: Update CLAUDE.md and README

Update documentation to reflect the new setup wizard and the Socket Mode → HTTP change.

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

**Step 1: Update CLAUDE.md**

- Remove `slack.app_token` / `SLACK_APP_TOKEN` from the configuration table
- Add `slack.signing_secret` / `SLACK_SIGNING_SECRET` to the table with description "Slack request signing secret (required)"
- Update Architecture section to mention the setup wizard
- Note that slash commands are received via HTTP POST, not Socket Mode

**Step 2: Update README.md**

- Add a "Quick Start" section describing the setup wizard experience
- Update the Slack setup instructions to reflect manifest-based app creation
- Remove references to Socket Mode / app-level tokens
- Document the `--setup-listen` flag

**Step 3: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: update for setup wizard and HTTP slash commands"
```

---

### Task 12: End-to-End Manual Smoke Test

Verify the full setup wizard flow works end to end.

**Step 1: Clean state**

```bash
make clean
rm -f config.toml zoom-notifier.db
```

**Step 2: Build and run**

```bash
make build
./bin/zoom-notifier
```

Expected: Logs "No configuration found. Setup wizard available at http://localhost:8888/setup"

**Step 3: Walk through the wizard**

Open http://localhost:8888/setup in a browser and complete each step:
1. Welcome → click Get Started
2. Enter a server URL
3. Accept or customize the generated admin API key
4. Enter a test Zoom webhook secret, verify the webhook URL is displayed
5. Verify the "Create Slack App" button link contains the manifest, enter test Slack values
6. Review advanced settings (accept defaults)
7. Review all values, click Save
8. Verify `config.toml` was created with correct values and `0600` permissions

**Step 4: Restart and verify normal mode**

```bash
./bin/zoom-notifier --config config.toml
```

Expected: Normal startup — no setup wizard, API routes mounted, health check responds.

**Step 5: Verify slash command endpoint**

```bash
curl -X POST http://localhost:8888/slack/commands \
  -d "team_id=T123&user_id=U456&channel_id=C789&text=help"
```

Expected: 401 (unsigned request). Confirms the endpoint exists and validates signatures.

---

## Dependency Summary

**New packages:** None. Uses only stdlib (`embed`, `html/template`, `crypto/hmac`, `crypto/sha256`, `encoding/json`, `net/http`, `net/url`), plus existing Chi router.

**Removed packages:** `github.com/gorilla/websocket` (indirect, via socketmode removal). `github.com/slack-go/slack` stays for `sender.go`.

## File Summary

| Action | Path |
|--------|------|
| Create | `internal/setup/setup.go` |
| Create | `internal/setup/setup_test.go` |
| Create | `internal/setup/keygen.go` |
| Create | `internal/setup/keygen_test.go` |
| Create | `internal/setup/configwriter.go` |
| Create | `internal/setup/configwriter_test.go` |
| Create | `internal/setup/manifest.go` |
| Create | `internal/setup/manifest_test.go` |
| Create | `internal/setup/handler.go` |
| Create | `internal/setup/handler_test.go` |
| Create | `internal/setup/templates/*.html` (8 files) |
| Create | `internal/setup/static/style.css` |
| Create | `internal/setup/static/setup.js` |
| Create | `internal/slack/http_commands.go` |
| Create | `internal/slack/http_commands_test.go` |
| Modify | `cmd/zoom-notifier/main.go` |
| Modify | `internal/config/config.go` |
| Modify | `CLAUDE.md` |
| Modify | `README.md` |
| Delete | `internal/slack/bot.go` |
