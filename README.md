# zoom-notifier

Multi-tenant service that receives Zoom webhook events and dispatches notifications to Slack channels and/or IRC. Includes a native Slack app with slash commands, interactive modals, and a spec-first REST API.

## Features

- **Multi-tenant**: Multiple Slack workspaces, each with their own Zoom account, subscriptions, and settings
- **Slack App**: OAuth install flow, slash commands with signing secret verification, interactive modals for admin tasks
- **Meeting Links**: Optional clickable join links with passcodes via Zoom Server-to-Server OAuth API
- **Meeting Filters**: Only notify on matching meeting topics (case-insensitive substring match)
- **Notification Settings**: Tenant-wide defaults for message suffix and meeting links, with per-filter overrides
- **Setup Wizard**: Web-based first-run wizard guides you through Zoom and Slack app configuration
- **Per-Tenant Setup**: Each workspace configures its own Zoom account via a web form
- **REST API**: Full CRUD for tenants, subscriptions, filters, and credentials with Swagger UI docs
- **IRC Relay**: Per-tenant IRC notification support with TLS
- **Meeting State**: Tracks active meetings and participants in SQLite

## Quick Start

### Build

```bash
make build
```

Requires Go 1.25+. No C compiler needed (pure Go SQLite via `modernc.org/sqlite`).

### Configure (Setup Wizard)

Run the binary with no config file — a web-based setup wizard launches automatically:

```bash
./zoom-notifier
# => No configuration found. Setup wizard available at http://localhost:8888/setup
```

The wizard walks you through:
1. Setting your server's public URL
2. Creating a Zoom Server-to-Server OAuth app (Account ID, webhook secret, optional API credentials)
3. Creating a Slack app (via pre-built manifest link) and entering credentials
4. Advanced settings (host, port, database path, log level, admin API key)

On completion it writes a `config.toml` file. Restart to apply.

For headless servers, use `--setup-listen 0.0.0.0:8888` to expose the wizard on all interfaces.

### Configure (Manual)

Create a config file directly:

```toml
[server]
port = 8888
host = "0.0.0.0"
url = "https://zoom.example.com"           # public URL for OAuth redirects

[database]
path = "./zoom-notifier.db"

[zoom]
webhook_secret = "your-zoom-secret-token"   # or ZOOM_SECRET env var
account_id = "your-zoom-account-id"

# Optional: enables meeting join links in notifications
# client_id = "your-zoom-client-id"
# client_secret = "your-zoom-client-secret"

[slack]
client_id = "your-slack-client-id"          # or SLACK_CLIENT_ID
client_secret = "your-slack-client-secret"  # or SLACK_CLIENT_SECRET
signing_secret = "your-slack-signing-secret" # or SLACK_SIGNING_SECRET

[admin]
api_key = "your-admin-key"                  # or ZOOMNOTIFIER_ADMIN_KEY

[log]
level = "info"
```

### Run

```bash
./zoom-notifier --config config.toml
```

### Flags

- `--version` -- Show version information
- `--config <path>` -- Path to TOML config file
- `--migrate` -- Run database migrations and exit
- `--setup-listen <host:port>` -- Listen address for setup wizard (default: `localhost:8888`)

## Zoom App Setup

Create a **Server-to-Server OAuth** app in the Zoom Marketplace. This single app handles both webhook events and API access:

1. Go to [Zoom App Marketplace - Build App](https://marketplace.zoom.us/user/build) and sign in
2. Choose **Server-to-Server OAuth**, give it a name (e.g. "zoom-notifier")
3. Copy **Account ID**, **Client ID**, and **Client Secret** from the App Credentials tab
4. Fill in the required fields on the Information tab
5. On the Feature tab, enable **Event Subscriptions** and add:
   - Event notification endpoint: `https://your-server/webhook/zoom`
   - Events: Start Meeting, End Meeting, Participant/Host Joined, Participant/Host Left
6. Copy the **Secret Token** from the Feature tab (this is the webhook secret, not the Client Secret)
7. On the Scopes tab, add `meeting:read:admin`
8. Activate the app

## Slack App Setup

The setup wizard provides a "Create Slack App from Manifest" button that pre-fills all settings. If setting up manually:

1. Create a Slack app at https://api.slack.com/apps
2. Add the `/zoom-notifier` slash command with request URL `https://your-server/slack/commands`
3. Enable Interactivity with request URL `https://your-server/slack/interactions`
4. Set OAuth scopes: `commands`, `chat:write`, `chat:write.public`, `channels:read`, `groups:read`, `im:write`, `im:read`
5. Set the OAuth redirect URL to `https://your-server/slack/callback`
6. Install the app via `https://your-server/slack/install`

The person who installs the app is automatically added as the first admin.

### Slash Commands

**Everyone:**

| Command | Description |
|---|---|
| `/zoom-notifier status` | Show active meetings with participants |
| `/zoom-notifier whois <search>` | List participants by topic (partial match) |
| `/zoom-notifier filters` | List active meeting filters |
| `/zoom-notifier settings` | Show notification settings, filter overrides, and subscriptions (admin) |
| `/zoom-notifier help` | Show available commands (admin commands shown to admins only) |

**Admin only:**

| Command | Description |
|---|---|
| `/zoom-notifier subscribe` | Subscribe a channel (opens channel picker modal) |
| `/zoom-notifier unsubscribe` | Unsubscribe a channel (opens dropdown modal) |
| `/zoom-notifier filter` | Add a meeting topic filter (opens modal) |
| `/zoom-notifier set-suffix <text>` | Set default message suffix |
| `/zoom-notifier set-suffix "Filter" "text"` | Set suffix override on a filter |
| `/zoom-notifier set-link on\|off` | Toggle default meeting links |
| `/zoom-notifier set-link "Filter" on\|off` | Toggle links on a filter |
| `/zoom-notifier admins list` | List admins |
| `/zoom-notifier admins add` | Add an admin (opens user picker modal) |
| `/zoom-notifier admins remove @user` | Remove an admin |
| `/zoom-notifier api-key` | Show tenant API key |
| `/zoom-notifier setup` | Open per-tenant Zoom credential setup page |

Commands with modals also accept text arguments as a fallback (e.g. `/zoom-notifier subscribe #channel`).

Short aliases: `sub`, `unsub`, `subs`, `admin`/`admins`.

## Per-Tenant Zoom Setup

Each Slack workspace configures its own Zoom account. After installing the Slack app:

1. An admin runs `/zoom-notifier setup` in Slack
2. This returns a private link to a web-based setup form
3. The form collects: Zoom Account ID (required), Client ID and Client Secret (optional, for meeting links)
4. Save -- the workspace is now connected to its Zoom account

## Notification Settings

Notifications use a two-tier configuration model:

- **Tenant defaults**: Message suffix and meeting link toggle apply to all notifications
- **Filter overrides**: Individual filters can override the suffix and/or link setting

Example: Tenant default suffix is "the Zoom meeting", but a filter for "Standup" has a suffix override of "the daily standup". Meetings matching "Standup" get the override; everything else gets the default.

## IRC Setup

Configure IRC via the REST API:

```bash
curl -X PUT https://your-server/api/v1/tenants/$TENANT_ID/irc \
  -H "Authorization: Bearer $TENANT_KEY" \
  -H "Content-Type: application/json" \
  -d '{"server": "irc.libera.chat:6697", "nick": "zoombot", "password": "secret", "use_tls": true}'
```

Then add an IRC subscription:

```bash
curl -X POST https://your-server/api/v1/tenants/$TENANT_ID/subscriptions \
  -H "Authorization: Bearer $TENANT_KEY" \
  -H "Content-Type: application/json" \
  -d '{"type": "irc", "target": "#mychannel"}'
```

## API Documentation

Interactive Swagger UI docs are available at `/api/docs` when the server is running.

The REST API is defined in `api/openapi.yaml` (OpenAPI 3.0). Key endpoints:

- `GET /healthz` -- Health check (version, uptime, database status, tenant count)
- `POST /webhook/zoom` -- Zoom webhook receiver
- `GET/POST /api/v1/tenants` -- Tenant management (admin key)
- `PUT /api/v1/tenants/{id}/defaults` -- Update tenant notification defaults
- `GET/POST/DELETE /api/v1/tenants/{id}/subscriptions` -- Subscription CRUD
- `GET/POST/PATCH/DELETE /api/v1/tenants/{id}/filters` -- Meeting filter management
- `GET/PUT /api/v1/tenants/{id}/irc` -- IRC configuration
- `PUT /api/v1/tenants/{id}/zoom` -- Zoom API credentials

## Web Pages

| Path | Description |
|---|---|
| `/` | Landing page with links |
| `/setup` | First-run setup wizard (only when unconfigured) |
| `/slack/install` | Slack OAuth install flow |
| `/tenant/setup?key=...` | Per-tenant Zoom credential setup |
| `/api/docs` | Swagger UI API documentation |
| `/healthz` | Health check endpoint |

## Running via systemd

See `contrib/zoom-notifier.service` for a sample systemd unit file.

## License

MIT
