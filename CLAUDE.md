# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
make build          # Build binary (runs fmt + tidy first, embeds version via ldflags)
make fmt            # Format code with go fmt
make tidy           # go mod tidy (runs fmt first)
make clean          # Remove built binary and bin/ directory
make platforms      # Cross-compile for all platforms (mac + linux)
make install        # Build and install to /usr/local/bin
```

There are no Go test files. Manual testing uses `examples/tests.sh` which POSTs sample JSON payloads from `examples/zoom/` to localhost:8889.

## CI

GitHub Actions runs on push/PR to main: `make build`, `go vet ./...`, and `golangci-lint`.

## Architecture

Single-binary Go webhook server that receives Zoom webhook events and dispatches notifications to Slack and/or IRC.

**Flow:** Zoom POST → Gin HTTP handler (`main.go:processWebHook`) → event filtering → `dispatchMessage()` → Slack/IRC backends

**Source files:**
- `main.go` — HTTP server (Gin), webhook processing, CRC validation, configuration (Viper), message dispatch routing
- `slack.go` — Slack webhook formatting and posting; supports multiple comma-separated webhook URIs
- `irc.go` — IRC connection, auth, and message sending via go-ircevent
- `zoom_api.go` — Optional Zoom OAuth2 + REST API integration to fetch meeting join links with passcodes

**Key dependencies:** Gin (HTTP), Viper (config), Logrus (logging), go-ircevent (IRC)

## Configuration

All configuration is via environment variables (bound through Viper):

- `ZOOM_SECRET` (required) — Webhook CRC validation token
- `ZOOMWH_PORT` (default: 8888) — HTTP listen port
- `ZOOMWH_SLACK_ENABLE` / `ZOOMWH_SLACK_WH_URI` — Slack backend
- `ZOOMWH_IRC_ENABLE` / `ZOOMWH_IRC_SERVER` / `ZOOMWH_IRC_CHANNEL` / `ZOOMWH_IRC_NICK` / `ZOOMWH_IRC_PASS` — IRC backend
- `ZOOM_API_ENABLE` / `ZOOM_API_CLIENT_ID` / `ZOOM_API_CLIENT_SECRET` / `ZOOM_API_ACCOUNT_ID` — Optional Zoom API for meeting links
- `ZOOMWH_MEETING_NAME` — Optional topic filter (exact match)
- `ZOOMWH_MSG_SUFFIX` (default: "the zoom meeting.") — Suffix for join/leave messages

## Handled Zoom Events

- `endpoint.url_validation` — CRC challenge-response
- `meeting.participant_joined` / `meeting.participant_left` — Dispatched to notification backends
- All other events are silently ignored
