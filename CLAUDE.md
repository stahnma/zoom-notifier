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
go test ./internal/slack/ -v    # Run Slack tests (OAuth + commands)
go test ./internal/store/sqlite/ -v  # Run store tests
```

Manual smoke testing uses `examples/tests.sh` which exercises the REST API and webhook endpoints.

## CI

GitHub Actions runs on push/PR to main: `make build`, `go vet ./...`, `go test ./internal/...`, and `golangci-lint`.

A separate deploy workflow builds linux-amd64, deploys via SCP, and restarts the systemd service.

## Architecture

Multi-tenant Go service that receives Zoom webhook events and dispatches notifications to Slack and/or IRC. Includes a native Slack app with HTTP slash commands (verified via signing secret), OAuth install flow, and a spec-first REST API. A web-based setup wizard launches automatically when no configuration exists.

```
cmd/zoom-notifier/main.go    # Entry point: config, wiring, startup, graceful shutdown
internal/
  config/                     # Viper-based config (TOML + env vars + defaults)
  store/                      # Store interface (types.go, store.go)
    sqlite/                   # SQLite implementation with golang-migrate migrations
  zoom/                       # Webhook handler, CRC validation, Zoom REST API client
  setup/                      # Web-based setup wizard (embedded templates, config writer)
  slack/                      # Slack sender, OAuth install flow, HTTP slash commands
  irc/                        # IRC relay (notification sink, connect-per-message)
  notify/                     # Notification dispatcher (fan-out to Slack/IRC per subscription)
  api/                        # Generated Chi server (oapi-codegen), auth middleware, server impl
api/
  openapi.yaml                # OpenAPI 3.0 spec (source of truth for REST API)
```

**Flow:** Zoom POST → webhook handler → tenant resolution → meeting state → dispatcher → matching subscriptions → Slack/IRC backends

**Slack App Flow:** HTTP POST /slack/commands → signing secret verification → slash command router → store operations → JSON response

**API Flow:** HTTP request → Chi router → oapi-codegen strict handler → auth middleware (AdminKey/TenantKey scopes) → server methods → store

## Key Design Decisions

- **Pure Go**: `CGO_ENABLED=0` with `modernc.org/sqlite` — no C compiler needed
- **Store interface**: `store.Store` interface enables future database swaps
- **Spec-first API**: OpenAPI 3.0 → oapi-codegen strict server mode → compile-time route/type safety
- **Security scopes**: Generated code sets `AdminKeyScopes`/`TenantKeyScopes` in context; single `AuthMiddleware` checks both
- **Multi-tenant**: Tenants created via REST API or Slack OAuth install; isolated subscriptions, filters, credentials

## Configuration

All configuration via TOML config file and/or environment variables:

| Config Key | Env Var | Description | Default |
|---|---|---|---|
| `server.port` | - | HTTP listen port | 8888 |
| `server.host` | - | HTTP listen host | localhost |
| `database.path` | - | SQLite database path | ./zoom-notifier.db |
| `zoom.webhook_secret` | `ZOOM_SECRET` | Zoom CRC validation token | (required) |
| `slack.client_id` | `SLACK_CLIENT_ID` | Slack app client ID | (required) |
| `slack.client_secret` | `SLACK_CLIENT_SECRET` | Slack app client secret | (required) |
| `slack.signing_secret` | `SLACK_SIGNING_SECRET` | Slack request signing secret | (required) |
| `admin.api_key` | `ZOOMNOTIFIER_ADMIN_KEY` | Admin API key for tenant management | (required) |
| `log.level` | - | Log level (debug/info/warn/error) | info |

## Handled Zoom Events

- `endpoint.url_validation` — CRC challenge-response
- `meeting.started` — Upsert meeting state
- `meeting.ended` — Clean up meeting and participants
- `meeting.participant_joined` — Track participant, dispatch notifications
- `meeting.participant_left` — Update participant, dispatch notifications
- All other events are silently ignored

## Slash Commands

| Command | Access | Description |
|---|---|---|
| `/zoom-notifier status` | everyone | Show active meetings |
| `/zoom-notifier whois <meeting>` | everyone | List participants |
| `/zoom-notifier subscribe #channel` | admin | Subscribe channel |
| `/zoom-notifier unsubscribe #channel` | admin | Unsubscribe channel |
| `/zoom-notifier filter "Topic"` | admin | Add meeting filter |
| `/zoom-notifier filters` | everyone | List active filters |
| `/zoom-notifier settings` | everyone | Show notification settings and filter overrides |
| `/zoom-notifier set-suffix "text"` | admin | Set default message suffix |
| `/zoom-notifier set-suffix "Filter" "text"` | admin | Set suffix override on a filter |
| `/zoom-notifier set-link on/off` | admin | Toggle default meeting links |
| `/zoom-notifier set-link "Filter" on/off` | admin | Toggle meeting links on a filter |
| `/zoom-notifier admins add @user` | admin | Add an admin |
| `/zoom-notifier api-key` | admin | Show tenant API key |
| `/zoom-notifier setup` | admin | Open per-tenant Zoom setup page |
| `/zoom-notifier help` | everyone | List commands |
