# Zoom Notifier Architecture Redesign

**Date:** 2026-02-24
**Status:** Approved

## Overview

Redesign zoom-notifier from a simple webhook relay into a multi-tenant, API-driven service with a native Slack app, per-tenant IRC support, persistent meeting state, and spec-first REST API. Single binary, single deployment.

## Architecture: Monolith with Embedded Slack Bot

Single Go binary handling Zoom webhook ingestion, Slack app (Socket Mode), IRC relay, REST API, and SQLite state.

```
                     +-------------------------------------+
  Zoom Webhooks ---> |          zoom-notifier              |
                     |                                     |
                     |  +-----------+  +---------------+   |
                     |  | Webhook   |  | Slack Bot     |   |
                     |  | Handler   |  | (Socket Mode) |   |
                     |  +-----+-----+  +-------+-------+   |
                     |        |                |           |
                     |        v                v           |
                     |  +-----------------------------+   |
                     |  |     Meeting State Engine     |   |
                     |  |        (SQLite)              |   |
                     |  +-------------+---------------+   |
                     |                |                    |
                     |     +----------+----------+        |
                     |     v          v          v        |
                     |  +------+ +--------+ +--------+   |
                     |  |Slack | |  IRC   | |REST API|   |
                     |  |Notify| | Relay  | |(Chi)   |   |
                     |  +------+ +--------+ +--------+   |
                     +-------------------------------------+
```

Socket Mode means Slack events arrive over an outbound WebSocket — no additional public endpoint needed. The only inbound HTTP traffic is Zoom webhooks and REST API calls.

## Module Structure

```
zoom-notifier/
├── cmd/zoom-notifier/        # main.go — startup, wiring, CLI flags
├── internal/
│   ├── config/               # Viper-based config (env + TOML + flags)
│   ├── store/                # SQLite via modernc.org/sqlite — tenants, subscriptions, meeting state
│   ├── zoom/                 # Webhook handler, CRC validation, Zoom REST API client (OAuth2)
│   ├── slack/                # Slack app: Socket Mode bot, slash commands, OAuth install flow
│   ├── irc/                  # IRC relay (notification sink only)
│   └── api/                  # Chi router, oapi-codegen handlers, REST API
├── api/
│   └── openapi.yaml          # Source-of-truth API spec
├── examples/
│   ├── zoom/                 # Sample Zoom webhook payloads (existing)
│   ├── slack/                # Sample Slack event payloads
│   └── api/                  # Sample API request/response payloads
├── docs/plans/
├── go.mod
├── Makefile
└── CLAUDE.md
```

### Startup Flow

1. Parse CLI flags + load Viper config (env -> TOML -> defaults)
2. Open SQLite database, run migrations (golang-migrate)
3. Start Slack Socket Mode connection (slash commands + interactive messages)
4. Start Chi HTTP server (Zoom webhooks + REST API)
5. IRC relay initialized lazily per-tenant when enabled

### Design Principle

The `store` package is the single source of truth. Zoom handlers write to it, Slack commands read/write to it, the REST API reads from it. No global state — everything flows through the store.

## Technology Choices

| Area | Choice | Rationale |
|---|---|---|
| Web framework | Chi | Lightweight, stdlib-compatible, pairs with oapi-codegen |
| API spec | OpenAPI 3.x, spec-first with oapi-codegen | Compile-time guarantee that spec and code stay in sync |
| Storage | SQLite via modernc.org/sqlite (pure Go, no CGO) | Persistent, zero ops, per-deployment isolation |
| Migrations | golang-migrate with SQL migration files | Works with SQLite now, Postgres/MySQL later |
| Config | Viper (env + TOML + CLI flags) | Already in use, supports runtime reloading |
| Config format | TOML | No significant-whitespace issues |
| Slack SDK | github.com/slack-go/slack with socketmode | Official community SDK, Socket Mode support |
| Logging | logrus (existing) | Already in use |

### Future-Proofing: Store Interface

The `store` package exposes a Go interface. SQLite implementation today; GORM/Postgres later without touching any other package.

```go
type Store interface {
    GetTenant(ctx context.Context, workspaceID string) (*Tenant, error)
    CreateSubscription(ctx context.Context, sub *Subscription) error
    GetMeetingParticipants(ctx context.Context, meetingID string) ([]Participant, error)
    // ...
}
```

## Data Model

```sql
-- Workspace tenants (one per Slack OAuth install or API-created)
tenants (
    id              TEXT PRIMARY KEY,  -- Slack workspace ID or generated UUID
    team_name       TEXT,
    bot_token       TEXT,              -- encrypted at rest, NULL for IRC-only tenants
    api_key         TEXT NOT NULL,     -- for REST API auth
    installed_at    DATETIME,
    zoom_account_id TEXT               -- links webhooks to this tenant
)

-- Who can change settings (everyone else is read-only)
tenant_admins (
    tenant_id     TEXT REFERENCES tenants(id),
    slack_user_id TEXT,
    PRIMARY KEY (tenant_id, slack_user_id)
)

-- Which channels get notified for which meetings
subscriptions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id    TEXT REFERENCES tenants(id),
    type         TEXT NOT NULL,         -- 'slack' or 'irc'
    meeting_id   TEXT,                  -- NULL = all meetings for this tenant
    target       TEXT NOT NULL,         -- Slack channel ID or IRC channel name
    msg_suffix   TEXT DEFAULT 'the zoom meeting.',
    include_link BOOLEAN DEFAULT true,
    enabled      BOOLEAN DEFAULT true,
    created_at   DATETIME
)

-- Topic filters (if set, only matching meetings dispatch)
meeting_filters (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id TEXT REFERENCES tenants(id),
    pattern   TEXT NOT NULL             -- exact match on meeting topic
)

-- Live meeting state (built from webhooks, cleared on meeting end)
active_meetings (
    meeting_id TEXT PRIMARY KEY,
    tenant_id  TEXT REFERENCES tenants(id),
    topic      TEXT,
    host_id    TEXT,
    start_time DATETIME,
    join_url   TEXT
)

-- Who is currently in each meeting
participants (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    meeting_id TEXT REFERENCES active_meetings(meeting_id),
    user_name  TEXT,
    email      TEXT,
    join_time  DATETIME,
    leave_time DATETIME                 -- NULL = still in meeting
)

-- Per-tenant IRC connection config
irc_configs (
    tenant_id TEXT REFERENCES tenants(id),
    server    TEXT NOT NULL,             -- host:port
    nick      TEXT NOT NULL,
    password  TEXT NOT NULL,             -- encrypted at rest
    use_tls   BOOLEAN DEFAULT true,
    PRIMARY KEY (tenant_id, server)
)
```

## Zoom Integration

### Webhook Handler (Zoom pushes to us)

- `POST /webhook/zoom` on Chi router
- CRC validation via HMAC-SHA256 with deployment-level `ZOOM_SECRET`
- Tenant resolution: `payload.account_id` -> lookup `tenants.zoom_account_id`
- Events handled:
  - `endpoint.url_validation` — CRC challenge-response
  - `meeting.started` / `meeting.ended` — create/clear `active_meetings` rows
  - `meeting.participant_joined` / `meeting.participant_left` — upsert `participants`, fan out to subscriptions

### Zoom REST API Client (we query Zoom)

- OAuth2 account credentials flow with token caching (current code fetches a new token every call)
- Per-tenant credentials stored encrypted in SQLite
- Endpoints used:
  - `GET /meetings/{id}` — fetch join URL with passcode
  - `GET /meetings/{id}/participants` — backfill participant list on restart
- Backfill on startup: query Zoom API for active meetings to hydrate state after restart

### Zoom Credential Ownership

Per-tenant. Each tenant provides their own Zoom OAuth credentials (Client ID, Secret, Account ID) because a single Zoom account cannot see other accounts' meetings. Setup is done via:
- Slack: `/zoom-notifier setup` guided flow
- API: `PUT /api/v1/tenants/{id}/zoom`

## Slack App

### OAuth Install Flow

- `GET /slack/install` — redirects to Slack OAuth authorize URL
- `GET /slack/callback` — exchanges code for bot token, creates tenant in SQLite
- Scopes: `commands`, `chat:write`, `channels:read`
- Posts welcome message with setup instructions after install

### Socket Mode Bot

Uses `github.com/slack-go/slack` with `socketmode` package. Persistent outbound WebSocket — no public endpoint needed for Slack events.

### Slash Commands

| Command | Who | What |
|---|---|---|
| `/zoom-notifier status` | everyone | Show active meetings and participant counts |
| `/zoom-notifier whois {meeting}` | everyone | List who's in a specific meeting |
| `/zoom-notifier subscribe #channel` | admin | Add a channel to receive notifications |
| `/zoom-notifier unsubscribe #channel` | admin | Remove a channel |
| `/zoom-notifier filter "Topic Name"` | admin | Add a meeting topic filter |
| `/zoom-notifier filters` | everyone | List active filters |
| `/zoom-notifier setup` | admin | Guided Zoom credential setup |
| `/zoom-notifier set-suffix #channel "the standup"` | admin | Set message suffix per subscription |
| `/zoom-notifier set-link #channel on/off` | admin | Toggle meeting link in notifications |
| `/zoom-notifier admins add @user` | admin | Grant admin to a user |
| `/zoom-notifier api-key` | admin | Generate/show tenant API key (ephemeral) |
| `/zoom-notifier help` | everyone | List available commands |

### Permission Model

- Installing user is automatically first admin
- Admins stored in `tenant_admins` table
- Mutations require admin; read-only commands available to everyone

### Notification Format

- Block Kit structured messages for Slack
- Join notifications include a "Join Meeting" button with passcode URL
- Message format: `"{username} has joined/left {msg_suffix}"` — preserves current behavior
- `msg_suffix` and `include_link` configurable per-subscription

## IRC Relay

- Per-tenant configuration stored in `irc_configs` table
- Configured via REST API (not Slack slash commands) — IRC users may not use Slack
- Notification sink only — no bot commands from IRC
- Connect-per-message pattern (no connection pooling)
- TLS support
- Enable/disable per-tenant

## Multi-Tenancy

Three onboarding paths:

### Slack-only tenant
1. Click "Add to Slack" -> OAuth flow -> tenant created

### Slack + IRC tenant
1. Install Slack app
2. Run `/zoom-notifier api-key` to get API key
3. `PUT /api/v1/tenants/{id}/irc` with IRC config

### IRC-only tenant
1. `POST /api/v1/tenants` with deployment admin key -> get tenant ID + API key
2. `PUT /api/v1/tenants/{id}/zoom` with Zoom credentials
3. `PUT /api/v1/tenants/{id}/irc` with IRC config
4. `POST /api/v1/tenants/{id}/subscriptions` to subscribe channels

Slack is a first-class tenant type but not a prerequisite.

### Tenant Isolation

Each tenant has its own: bot token, Zoom credentials, subscriptions, filters, admin list, IRC config. No shared secrets between tenants. The deployment-level secrets are only `ZOOM_SECRET` (webhook CRC) and `ZOOMNOTIFIER_ADMIN_KEY` (tenant creation).

## REST API

Spec-first with `oapi-codegen` generating Chi server interfaces.

### Authentication

- Zoom webhook: unauthenticated (CRC/HMAC validated)
- Tenant endpoints: `Authorization: Bearer {api_key}`, scoped to that tenant
- Tenant creation: `Authorization: Bearer {admin_key}` (deployment-level)

### Endpoints

```
# Health
GET  /healthz

# Zoom webhook ingest
POST /webhook/zoom

# Tenant management (admin key)
POST /api/v1/tenants
GET  /api/v1/tenants

# Tenant config (tenant API key)
GET    /api/v1/tenants/{id}
DELETE /api/v1/tenants/{id}

# Zoom credentials (tenant API key)
PUT /api/v1/tenants/{id}/zoom

# Subscriptions (tenant API key)
GET    /api/v1/tenants/{id}/subscriptions
POST   /api/v1/tenants/{id}/subscriptions
PATCH  /api/v1/tenants/{id}/subscriptions/{sid}
DELETE /api/v1/tenants/{id}/subscriptions/{sid}

# IRC config (tenant API key)
GET /api/v1/tenants/{id}/irc
PUT /api/v1/tenants/{id}/irc

# Meeting filters (tenant API key)
GET    /api/v1/tenants/{id}/filters
POST   /api/v1/tenants/{id}/filters
DELETE /api/v1/tenants/{id}/filters/{fid}

# Meeting state (tenant API key)
GET /api/v1/tenants/{id}/meetings
GET /api/v1/tenants/{id}/meetings/{mid}/participants

# Admin management (tenant API key, must be admin)
GET    /api/v1/tenants/{id}/admins
POST   /api/v1/tenants/{id}/admins
DELETE /api/v1/tenants/{id}/admins/{uid}

# API key rotation (tenant API key)
POST /api/v1/tenants/{id}/rotate-key
```

Slash commands are thin wrappers — `/zoom-notifier subscribe #channel` calls the same store methods as `POST /api/v1/tenants/{id}/subscriptions`.

## Configuration & Deployment

### Viper Config Hierarchy (highest priority wins)

1. CLI flags
2. Environment variables
3. TOML config file
4. Defaults

### Config File

```toml
# /etc/zoom-notifier/config.toml (production)
# or ./config.dev.toml (local development)

[server]
port = 8888
host = "localhost"

[database]
path = "/var/lib/zoom-notifier/data.db"
# encryption_key via env: ZOOMNOTIFIER_DB_KEY

[zoom]
# webhook_secret via env: ZOOM_SECRET

[slack]
# client_id via env: SLACK_CLIENT_ID
# client_secret via env: SLACK_CLIENT_SECRET
# app_token via env: SLACK_APP_TOKEN
# signing_secret via env: SLACK_SIGNING_SECRET

[admin]
# api_key via env: ZOOMNOTIFIER_ADMIN_KEY

[log]
level = "info"
```

Config file search order: `--config` flag, `./`, `~/.config/zoom-notifier/`, `/etc/zoom-notifier/`.

### CLI Flags

```
zoom-notifier --version          # print version and exit
zoom-notifier --config path.toml # config file location
zoom-notifier --migrate          # run DB migrations and exit
```

### Local Development

No root or system-level setup required:
- SQLite defaults to `./zoom-notifier.db` in current directory
- Server binds to `localhost:8888`
- Config found in current directory
- Migrations run automatically on startup

```bash
# Minimal local run
ZOOM_SECRET=devsecret ZOOMNOTIFIER_ADMIN_KEY=devkey ./zoom-notifier

# Or with config file
./zoom-notifier --config config.dev.toml
```

### GitHub Actions Deployment

```
on push to main:
  1. make build (linux-amd64)
  2. go vet + golangci-lint
  3. go test ./...
  4. SCP binary to server
  5. SSH: install binary, restart systemd service
  6. Health check: curl /healthz
```

Secrets: `DEPLOY_HOST`, `DEPLOY_SSH_KEY`, `DEPLOY_USER`.

### Systemd Unit

```ini
[Unit]
Description=Zoom Notifier
After=network.target

[Service]
User=zoom-notifier
ExecStart=/usr/local/bin/zoom-notifier --config /etc/zoom-notifier/config.toml
EnvironmentFile=/etc/zoom-notifier/secrets.env
Restart=always

[Install]
WantedBy=multi-user.target
```

## Sample Payloads

```
examples/
├── zoom/
│   ├── endpoint_url_validation.json      # existing
│   ├── meeting_started.json              # existing
│   ├── meeting_ended.json                # existing
│   ├── participant_joined.json           # existing
│   ├── participant_left.json             # existing
│   ├── participant_joined_goodtopic.json # existing
│   ├── participant_special_meeting.json  # existing
│   └── invalid_zoom_example.json         # existing
├── slack/
│   ├── slash_command.json
│   ├── oauth_callback.json
│   ├── interactive_message.json
│   └── socket_mode_event.json
├── api/
│   ├── create_tenant_request.json
│   ├── create_tenant_response.json
│   ├── create_subscription_request.json
│   └── irc_config_request.json
└── tests.sh
```

Existing Zoom samples preserved. New samples serve as both reference documentation and test fixtures.

## Testing Strategy

### Unit Tests

- Store methods against SQLite `:memory:`
- Webhook parsing and CRC validation with sample JSON from `examples/zoom/`
- Message formatting (suffix, link inclusion)
- Permission checks (admin vs. read-only)
- Subscription fan-out logic

### Integration Tests

- oapi-codegen generates a test client from the OpenAPI spec
- Real Chi router with in-memory SQLite store
- POST sample webhooks, verify correct subscriptions triggered
- Full API CRUD flows (create tenant -> add subscription -> receive webhook -> verify dispatch)
- Slack and Zoom API calls mocked via `httptest.Server`

### Makefile Targets

```makefile
test:
    go test ./...

test-verbose:
    go test -v ./...

test-coverage:
    go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

dev: build
    ./zoom-notifier --config config.dev.toml
```
