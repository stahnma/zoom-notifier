# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
make build          # Build binary (runs fmt + tidy first, embeds version via ldflags)
make test           # Run all tests
make test-verbose   # Run tests with verbose output
make test-coverage  # Run tests with coverage report
make generate       # Regenerate API code from OpenAPI spec
make fmt            # Format code with go fmt
make tidy           # go mod tidy (runs fmt first)
make dev            # Build and run with config.dev.toml
make clean          # Remove built binary, bin/, coverage.out
make platforms      # Cross-compile for all platforms (mac + linux)
make install        # Build and install to /usr/local/bin
```

Build requires `CGO_ENABLED=0` in environments without gcc (uses pure-Go SQLite via modernc.org/sqlite).

## Testing

```bash
go test ./internal/...          # Run all tests
go test ./internal/api/ -v      # Run API tests
go test ./internal/slack/ -v    # Run Slack tests (OAuth, commands, modals, interactions)
go test ./internal/store/sqlite/ -v  # Run store tests
go test ./internal/notify/ -v   # Run dispatcher tests
go test ./internal/setup/ -v    # Run setup wizard tests
```

Manual smoke testing uses `examples/tests.sh` which exercises the REST API and webhook endpoints.

## CI

GitHub Actions runs on push/PR to main: `make build`, `go vet ./...`, `go test ./internal/...`, and `golangci-lint`.

A separate deploy workflow builds linux-amd64, deploys via SCP, and restarts the systemd service.

## Architecture

Multi-tenant Go service that receives Zoom webhook events and dispatches notifications to Slack and/or IRC. Includes a native Slack app with HTTP slash commands (verified via signing secret), interactive modals, OAuth install flow, per-tenant Zoom setup, and a spec-first REST API. A web-based setup wizard launches automatically when no configuration exists.

```
cmd/zoom-notifier/main.go    # Entry point: config, wiring, startup, graceful shutdown, landing page
internal/
  config/                     # Viper-based config (TOML + env vars + defaults)
  store/                      # Store interface (types.go, store.go)
    sqlite/                   # SQLite implementation with golang-migrate migrations
  zoom/                       # Webhook handler, CRC validation, Zoom REST API client
  setup/                      # Web-based first-run setup wizard (embedded templates, config writer)
  slack/                      # Slack sender, OAuth install flow, slash commands, modals, interactions, tenant setup
  irc/                        # IRC relay (notification sink, connect-per-message)
  notify/                     # Notification dispatcher (fan-out to Slack/IRC, meeting link resolution)
  api/                        # Generated Chi server (oapi-codegen), auth middleware, server impl
api/
  openapi.yaml                # OpenAPI 3.0 spec (source of truth for REST API)
  embed.go                    # Embeds openapi.yaml into the binary
```

**Flow:** Zoom POST → webhook handler → tenant resolution (by Zoom Account ID) → meeting state → dispatcher → filter match → resolve suffix/link settings (filter override → tenant default) → fetch meeting join link (if enabled) → matching subscriptions → Slack/IRC backends

**Slack App Flow:** HTTP POST /slack/commands → signing secret verification → slash command router → modal opening (if trigger_id present) or text response → store operations → JSON response

**Slack Interaction Flow:** HTTP POST /slack/interactions → signing secret verification → view_submission routing by callback_id → store operations → confirmation message → 200 OK

**API Flow:** HTTP request → Chi router → oapi-codegen strict handler → auth middleware (AdminKey/TenantKey scopes) → server methods → store

## Key Design Decisions

- **Pure Go**: `CGO_ENABLED=0` with `modernc.org/sqlite` — no C compiler needed
- **Store interface**: `store.Store` interface enables future database swaps
- **Spec-first API**: OpenAPI 3.0 → oapi-codegen strict server mode → compile-time route/type safety
- **Security scopes**: Generated code sets `AdminKeyScopes`/`TenantKeyScopes` in context; single `AuthMiddleware` checks both
- **Multi-tenant**: Tenants created via Slack OAuth install; isolated subscriptions, filters, credentials, and Zoom accounts
- **Notification settings**: Two-tier model — tenant-wide defaults with per-filter overrides for message suffix and meeting link toggle
- **Slack modals**: Admin commands (subscribe, filter, set-suffix, set-link, admins add) open interactive modals with proper form UIs; text-based fallback when modals unavailable
- **Single Zoom app**: Server-to-Server OAuth app handles both webhooks and API access (no separate Webhook Only app needed)

## Configuration

All configuration via TOML config file and/or environment variables:

| Config Key | Env Var | Description | Default |
|---|---|---|---|
| `server.port` | - | HTTP listen port | 8888 |
| `server.host` | - | HTTP listen host | localhost |
| `server.url` | - | Public URL (for OAuth redirects, setup links) | (recommended) |
| `database.path` | - | SQLite database path | ./zoom-notifier.db |
| `zoom.webhook_secret` | `ZOOM_SECRET` | Zoom CRC validation token (Secret Token) | (required) |
| `zoom.account_id` | - | Zoom Account ID (links webhooks to tenant) | (required) |
| `zoom.client_id` | - | Zoom S2S OAuth Client ID (for meeting links) | (optional) |
| `zoom.client_secret` | - | Zoom S2S OAuth Client Secret | (optional) |
| `slack.client_id` | `SLACK_CLIENT_ID` | Slack app client ID | (required) |
| `slack.client_secret` | `SLACK_CLIENT_SECRET` | Slack app client secret | (required) |
| `slack.signing_secret` | `SLACK_SIGNING_SECRET` | Slack request signing secret | (required) |
| `admin.api_key` | `ZOOMNOTIFIER_ADMIN_KEY` | Admin API key for tenant management | (required) |
| `log.level` | - | Log level (debug/info/warn/error) | info |
| `log.format` | - | Log format (`text` or `json`) | text |

## Handled Zoom Events

- `endpoint.url_validation` — CRC challenge-response (also works during setup wizard)
- `meeting.started` — Upsert meeting state
- `meeting.ended` — Clean up meeting and participants
- `meeting.participant_joined` — Track participant, dispatch notifications
- `meeting.participant_left` — Update participant, dispatch notifications
- All other events are silently ignored

## Slack OAuth Scopes

`commands`, `chat:write`, `chat:write.public`, `channels:read`, `groups:read`, `im:write`, `im:read`

## Slash Commands

| Command | Access | Description |
|---|---|---|
| `/zoom-notifier status` | everyone | Show active meetings with participants |
| `/zoom-notifier whois <search>` | everyone | List participants by topic (partial match) |
| `/zoom-notifier filters` | everyone | List active filters |
| `/zoom-notifier settings` | everyone | Show notification settings, filter overrides, subscriptions (admin) |
| `/zoom-notifier help` | everyone | List commands (admin commands shown to admins only) |
| `/zoom-notifier subscribe` | admin | Subscribe channel (modal with channel picker) |
| `/zoom-notifier unsubscribe` | admin | Unsubscribe channel (modal with dropdown) |
| `/zoom-notifier subscriptions` | admin | List channel subscriptions |
| `/zoom-notifier filter` | admin | Add meeting filter (modal) |
| `/zoom-notifier set-suffix "text"` | admin | Set default message suffix |
| `/zoom-notifier set-suffix "Filter" "text"` | admin | Set suffix override on a filter |
| `/zoom-notifier set-link on/off` | admin | Toggle default meeting links |
| `/zoom-notifier set-link "Filter" on/off` | admin | Toggle meeting links on a filter |
| `/zoom-notifier admins list` | admin | List admins |
| `/zoom-notifier admins add` | admin | Add admin (modal with user picker, sends DM notification) |
| `/zoom-notifier admins remove @user` | admin | Remove admin (cannot remove self) |
| `/zoom-notifier api-key` | admin | Show tenant API key |
| `/zoom-notifier setup` | admin | Open per-tenant Zoom setup page |

Aliases: `sub`/`subscribe`, `unsub`/`unsubscribe`, `subs`/`subscriptions`, `admin`/`admins`


## Go Code Style Rules
- NEVER use `_` to discard errors. Always handle errors explicitly with a return, log, or wrap.
- Prefer `fmt.Errorf("context: %w", err)` for error wrapping.
- Follow effective Go: https://go.dev/doc/effective_go
- golangci-lint fixes must address the root cause, not suppress the symptom.
- When fixing lint errors, explain *why* the original code was problematic.

## Web Pages

| Path | Description |
|---|---|
| `/` | Landing page (version, links, endpoints) |
| `/setup` | First-run setup wizard (only in setup mode) |
| `/slack/install` | Slack OAuth install flow |
| `/tenant/setup?key=...` | Per-tenant Zoom credential setup |
| `/api/docs` | Swagger UI API documentation |
| `/api/docs/openapi.yaml` | Raw OpenAPI spec (embedded in binary) |
| `/healthz` | Health check (version, uptime, database status, tenant count) |
| `/livez` | Liveness probe (always 200 if process is running) |
| `/readyz` | Readiness probe (200 if database connected, 503 otherwise) |
| `/metrics` | Prometheus metrics (request counts, latency, webhook/notification stats) |
