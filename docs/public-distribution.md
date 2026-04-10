# Public Distribution Guide

This document covers what's needed to allow other companies/workspaces to install zoom-notifier.

## Current State

The app is architected for multi-tenant use — each Slack workspace that installs creates its own tenant with independent subscriptions, filters, settings, and Zoom account configuration. However, the Slack app is currently configured as an internal (single-workspace) app.

## Slack App Configuration

### Enable Public Distribution

1. Go to your Slack app at [api.slack.com/apps](https://api.slack.com/apps)
2. Navigate to **Manage Distribution**
3. Under "Distribute App to Other Workspaces", complete the checklist:
   - All required app information fields filled in
   - No hard-coded redirect URLs (zoom-notifier uses config-based URLs, so this is already handled)
   - Privacy policy URL
   - Terms of service URL
4. Toggle **"Distribute App to Other Workspaces"** on

### Slack App Directory (Optional)

If you want the app to be discoverable in Slack's app directory:

1. Submit for Slack's app review process
2. Requires additional documentation and compliance
3. Not required for distribution — you can share the install link directly without a directory listing

## Code Changes Needed

### Required

- **Token encryption at rest** — Bot tokens are currently stored in plain text in SQLite. The `database.encryption_key` config field exists but isn't implemented for token encryption yet. For a multi-tenant production service hosting other companies' tokens, encryption at rest is essential.

### Recommended

- **"Add to Slack" button** — Add a public-facing install page with Slack's standard [Add to Slack button](https://api.slack.com/docs/slack-button). Currently the install entry point is just the `/slack/install` redirect endpoint.

- **Rate limiting** — No rate limiting on the API or webhook endpoints. A misbehaving tenant or external abuse could affect all tenants. Consider per-tenant rate limits on the API and global rate limits on the webhook endpoint.

- **Monitoring and alerting** — The `/healthz` endpoint provides basic status. For a production multi-tenant service, add metrics (Prometheus), structured log aggregation, and alerting on error rates.

- **Tenant isolation audit** — The auth middleware scopes API access by tenant API key, so tenants should not be able to access each other's data. A formal security review of all endpoints would be prudent before hosting other companies' data.

- **Onboarding email/notification** — After install, the success page shows next steps and the `/zoom-notifier setup` command guides Zoom configuration. Consider adding a welcome DM from the bot to the installing user with getting-started instructions.

### Nice to Have

- **Usage dashboard** — A web UI showing per-tenant stats (active meetings, notifications sent, etc.)
- **Tenant self-service** — Allow tenant admins to delete their own tenant / uninstall via slash command
- **Billing integration** — If offering as a paid service, integrate with Stripe or similar for per-tenant billing
- **Custom branding** — Allow tenants to customize the bot name/icon (Slack supports this per-message via `username` and `icon_url` parameters)

## Architecture Readiness

What's already multi-tenant ready:

| Component | Status |
|---|---|
| OAuth install flow | Creates independent tenant per workspace |
| Per-tenant Zoom setup | `/zoom-notifier setup` → web form per workspace |
| Subscriptions | Scoped per tenant |
| Filters + settings | Scoped per tenant with per-filter overrides |
| Admin management | Per-tenant admin list |
| Webhook routing | Routes by Zoom Account ID → tenant |
| API authentication | Per-tenant API keys |
| Slash commands | Scoped to invoking workspace's tenant |
| Meeting state | Scoped per tenant |

## Deployment Considerations

- **Database** — SQLite works for low-to-moderate scale. For many tenants with high webhook volume, consider migrating to PostgreSQL (the `store.Store` interface makes this straightforward).
- **Horizontal scaling** — Single binary with SQLite can't scale horizontally. PostgreSQL + multiple instances behind a load balancer would be needed for high availability.
- **Backup** — SQLite database should be backed up regularly. The database contains bot tokens and API keys.
- **TLS** — The server should always be behind a TLS-terminating reverse proxy in production.
