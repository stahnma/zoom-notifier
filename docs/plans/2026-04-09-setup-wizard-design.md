# Setup Wizard Design

**Date:** 2026-04-09
**Status:** Draft

## Problem

First-time setup requires manually creating a TOML config file, knowing which environment variables to set, and understanding where to obtain credentials from Zoom and Slack. This is too much upfront friction for new users.

## Solution

A web-based setup wizard that launches automatically when no configuration exists. The wizard walks the user through each required value with inline help, generates a Slack app via manifest link, and writes a TOML config file.

Advanced users can skip the wizard entirely by creating the config file themselves.

## Setup Mode Detection

On startup, after attempting to load config, the server checks whether required values are present (admin API key, Zoom webhook secret, Slack client ID). If any are missing and no valid config file exists, the server starts in **setup mode** instead of normal mode.

In setup mode:
- Only the setup wizard routes (`/setup/*`) and static assets are mounted
- No API, webhook, Slack, or health routes are available
- Binds to `localhost:8888` by default
- Logs: `No configuration found. Setup wizard available at http://localhost:8888/setup`

A `--setup-listen` flag allows overriding the bind address (e.g., `--setup-listen 0.0.0.0:9090`) for headless servers where the user accesses the wizard from a remote browser. This flag is ignored once configuration exists.

> **Warning:** Exposing setup mode on `0.0.0.0` transmits secrets in the clear unless behind a reverse proxy with TLS.

Once setup completes and the user restarts, normal startup runs. The setup routes are never mounted when a valid config exists.

## Wizard Steps

### Step 1 — Welcome

Brief explanation of what zoom-notifier does and what the setup will configure. Checklist of prerequisites:

- A Zoom developer account
- Slack workspace admin access

### Step 2 — Server URL

The public-facing URL of this server (e.g., `https://zoom-notifier.example.com`). Used to:

- Pre-fill the Slack app manifest with the correct OAuth redirect URI and slash command request URL
- Display the Zoom webhook endpoint URL for the user to paste into the Zoom console

Help toggle explains when to use localhost vs a real domain.

### Step 3 — Admin API Key

Auto-generates a cryptographically secure random key and displays it. The user can accept the generated key or type their own.

Help toggle explains what the admin API key controls (tenant management via the REST API).

### Step 4 — Zoom Configuration

Single field: **Zoom Webhook Secret**.

Also displays the webhook endpoint URL (`{server_url}/webhook/zoom`) so the user can copy it into the Zoom developer console.

Help toggle provides step-by-step instructions:
1. Go to the Zoom App Marketplace > Manage > Your Apps
2. Select your app (or create a Server-to-Server OAuth app)
3. Under "Feature," enable Event Subscriptions
4. Set the Event notification endpoint URL to the displayed webhook URL
5. Copy the **Secret Token** value and paste it here

### Step 5 — Slack Configuration

This step uses a **Slack App Manifest** to automate most of the Slack app setup.

A "Create Slack App" button opens a link to `api.slack.com/apps?new_app=1&manifest_json=...` with the following pre-configured:

- Bot token scopes: `commands`, `chat:write`, `channels:read`
- Slash command: `/zoom-notifier` with request URL pointing to this server
- OAuth redirect URL: `{server_url}/slack/callback`

The user creates the app in Slack's UI with one click, then copies back three values:

- **Client ID**
- **Client Secret**
- **Signing Secret**

Help toggles on each field show exactly where to find the value on the Slack app settings page.

> **Note:** This design drops Socket Mode in favor of HTTP-based slash commands, since the server already requires a public URL for Zoom webhooks. This eliminates the App-Level Token requirement and simplifies both setup and runtime architecture. Slash command requests are verified using the Signing Secret.

### Step 6 — Advanced Settings (collapsed)

Expandable section, collapsed by default. All fields pre-filled with sensible defaults:

| Field | Default |
|---|---|
| Server host | `localhost` |
| Server port | `8888` |
| Database path | `./zoom-notifier.db` |
| Log level | `info` |

### Step 7 — Review & Save

- Summary of all configured values (secrets partially masked)
- **"Save Configuration"** button writes the TOML file
- Expandable **"Environment Variables"** section with copyable `export` commands for Docker/systemd deployments
- Restart instructions: "Configuration saved to `./config.toml`. Restart zoom-notifier to apply."

## Implementation

### Embedded Assets

HTML templates, CSS, and vanilla JS are embedded into the binary using Go's `embed` package. No external files to distribute — the single binary stays self-contained.

### Package Structure

New `internal/setup/` package:

```
internal/setup/
  setup.go          # Setup mode detection, HTTP handlers, config writing
  templates/        # Go HTML templates for each wizard step
  static/           # Minimal CSS and vanilla JS (toggles, expand/collapse)
```

### Integration with main.go

```go
cfg, err := config.Load(*configPath)
if setup.NeedsSetup(cfg) {
    setup.StartServer(listenAddr, *configPath)
    return
}
// ... normal startup continues
```

### Config File Output

The wizard writes a TOML file using a Go template (the output structure is simple enough that a TOML library is unnecessary). The file is created with `0600` permissions (owner read/write only) since it contains secrets.

Output path: `./config.toml` by default, or the path from `--config` if provided.

### Slack Manifest Generation

The manifest is a Go template that interpolates the server URL into redirect URI and request URL fields. Rendered as URL-encoded JSON appended to the Slack "create app from manifest" link.

### Dropping Socket Mode

Slash commands move from Socket Mode (WebSocket) to HTTP POST. This requires:

- New HTTP handler for `/slack/commands` that receives Slack slash command payloads
- Request verification using the Slack Signing Secret (already in config, currently unused)
- Removal of `internal/slack/bot.go` and Socket Mode dependency
- Updating the Slack app manifest to set the slash command request URL

The existing `CommandHandler` logic in `internal/slack/commands.go` can be reused — only the transport layer changes.

## Security

- **Local-only by default:** Setup mode binds to `localhost` only. The `--setup-listen` flag is required to expose it on other interfaces.
- **No re-entry:** Setup routes are never mounted when a valid config exists. To reconfigure, edit the TOML file or delete it and restart.
- **Input validation:** Each step validates before advancing (non-empty, format checks, minimum key length).
- **No external transmission:** Secrets are only POST'd to the local server. The Slack manifest link contains no secrets.
- **File permissions:** Config file written with `0600`.

## Future Considerations

### Base Path Prefix

Support mounting zoom-notifier at a subdirectory (e.g., `https://example.com/zoom/`). Would require prefixing all route registrations, updating Slack/Zoom URLs, and ensuring internal redirects respect the prefix.

### REST API for Settings Management

Expose configuration management via the REST API, enabling integrations like Hubot modules to manage subscriptions and settings programmatically without relying on Slack slash commands.

### Admin Web UI for Tenant Management

A browser-based interface for ongoing administration — managing tenants, viewing subscriptions, monitoring active meetings — beyond the one-time setup wizard. This would be a separate feature from the setup wizard, likely behind admin API key authentication.
