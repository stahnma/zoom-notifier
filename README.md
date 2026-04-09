# zoom-notifier

Multi-tenant service that receives Zoom webhook events and dispatches notifications to Slack channels and/or IRC. Includes a native Slack app with slash commands for self-service management.

## Features

- **Multi-tenant**: Supports multiple Slack workspaces and IRC configurations
- **Slack App**: OAuth install flow, HTTP slash commands with signing secret verification
- **IRC Relay**: Per-tenant IRC notification support with TLS
- **REST API**: Full CRUD API for managing tenants, subscriptions, filters, and credentials
- **Meeting State**: Tracks active meetings and participants in SQLite
- **Meeting Filters**: Only notify on matching meeting topics
- **Per-channel Config**: Customizable message suffix and meeting link toggle per subscription

## Quick Start

### Build

```bash
make build
```

Requires Go 1.25+. No C compiler needed (pure Go SQLite).

### Configure (Setup Wizard)

Just run the binary with no config file — a web-based setup wizard launches automatically:

```bash
./zoom-notifier
# => No configuration found. Setup wizard available at http://localhost:8888/setup
```

The wizard walks you through:
1. Setting your server's public URL
2. Generating an admin API key
3. Configuring your Zoom webhook secret
4. Creating a Slack app (via pre-built manifest link) and entering credentials
5. Optional advanced settings (host, port, database path, log level)

On completion it writes a `config.toml` file. Restart to apply.

For headless servers without a local browser, use `--setup-listen 0.0.0.0:8888` to expose the wizard on all interfaces.

### Configure (Manual)

Advanced users can skip the wizard and create a config file directly (see `config.dev.toml` for example):

```toml
[server]
port = 8888
host = "0.0.0.0"

[database]
path = "./zoom-notifier.db"

[zoom]
webhook_secret = "your-zoom-webhook-secret"  # or set ZOOM_SECRET env var

[slack]
client_id = "your-slack-client-id"            # or SLACK_CLIENT_ID
client_secret = "your-slack-client-secret"    # or SLACK_CLIENT_SECRET
signing_secret = "your-slack-signing-secret"  # or SLACK_SIGNING_SECRET

[admin]
api_key = "your-admin-key"                    # or ZOOMNOTIFIER_ADMIN_KEY

[log]
level = "info"
```

### Run

```bash
./zoom-notifier --config config.toml
```

### Flags

- `--version` — Show version information
- `--config <path>` — Path to TOML config file
- `--migrate` — Run database migrations and exit
- `--setup-listen <host:port>` — Listen address for setup wizard (default: `localhost:8888`)

## Setup

### 1. Create a Tenant

Using the admin API key:

```bash
curl -X POST http://localhost:8888/api/v1/tenants \
  -H "Authorization: Bearer $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{"team_name": "My Team", "zoom_account_id": "abc123"}'
```

Or install the Slack app via `/slack/install` (creates tenant automatically).

### 2. Add a Subscription

Using the tenant's API key (returned from tenant creation):

```bash
curl -X POST http://localhost:8888/api/v1/tenants/$TENANT_ID/subscriptions \
  -H "Authorization: Bearer $TENANT_KEY" \
  -H "Content-Type: application/json" \
  -d '{"type": "slack", "target": "#general"}'
```

Or use the Slack slash command: `/zoom-notifier subscribe #general`

### 3. Configure Zoom Webhook

Point your Zoom app's webhook URL to `http://your-host:8888/webhook/zoom`.

## Slack App Setup

The setup wizard automates most of this by providing a "Create Slack App" button with a pre-built manifest. If setting up manually:

1. Create a Slack app at https://api.slack.com/apps using the app manifest (or manually)
2. Add the `/zoom-notifier` slash command with request URL `http://your-host:8888/slack/commands`
3. Set OAuth scopes: `commands`, `chat:write`, `channels:read`
4. Set the OAuth redirect URL to `http://your-host:8888/slack/callback`
5. Configure: `SLACK_CLIENT_ID`, `SLACK_CLIENT_SECRET`, `SLACK_SIGNING_SECRET`

### Slash Commands

| Command | Description |
|---|---|
| `/zoom-notifier status` | Show active meetings and participant counts |
| `/zoom-notifier whois <meeting>` | List who's in a specific meeting |
| `/zoom-notifier subscribe #channel` | Subscribe a channel to notifications |
| `/zoom-notifier unsubscribe #channel` | Unsubscribe a channel |
| `/zoom-notifier filter "Topic"` | Only notify for matching meeting topics |
| `/zoom-notifier filters` | List active filters |
| `/zoom-notifier help` | Show all available commands |

Admin commands (subscribe, filter, etc.) require the user to be a tenant admin.

## IRC Setup

Configure IRC via the REST API:

```bash
curl -X PUT http://localhost:8888/api/v1/tenants/$TENANT_ID/irc \
  -H "Authorization: Bearer $TENANT_KEY" \
  -H "Content-Type: application/json" \
  -d '{"server": "irc.libera.chat:6697", "nick": "zoombot", "password": "secret", "use_tls": true}'
```

Then add an IRC subscription:

```bash
curl -X POST http://localhost:8888/api/v1/tenants/$TENANT_ID/subscriptions \
  -H "Authorization: Bearer $TENANT_KEY" \
  -H "Content-Type: application/json" \
  -d '{"type": "irc", "target": "#mychannel"}'
```

## API Documentation

The REST API is defined in `api/openapi.yaml` (OpenAPI 3.0). Key endpoints:

- `GET /healthz` — Health check
- `POST /webhook/zoom` — Zoom webhook receiver
- `GET/POST /api/v1/tenants` — Tenant management (admin key)
- `GET/POST/PATCH/DELETE /api/v1/tenants/{id}/subscriptions` — Subscription CRUD
- `GET/POST/DELETE /api/v1/tenants/{id}/filters` — Meeting filter management
- `GET/PUT /api/v1/tenants/{id}/irc` — IRC configuration
- `PUT /api/v1/tenants/{id}/zoom` — Zoom API credentials

## Running via systemd

See `contrib/zoom-notifier.service` for a sample systemd unit file.

## License

MIT
