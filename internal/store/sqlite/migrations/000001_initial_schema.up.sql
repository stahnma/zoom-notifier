CREATE TABLE IF NOT EXISTS tenants (
    id              TEXT PRIMARY KEY,
    team_name       TEXT,
    bot_token       TEXT,
    api_key         TEXT NOT NULL,
    installed_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    zoom_account_id TEXT
);

CREATE TABLE IF NOT EXISTS tenant_admins (
    tenant_id     TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slack_user_id TEXT NOT NULL,
    PRIMARY KEY (tenant_id, slack_user_id)
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type         TEXT NOT NULL CHECK(type IN ('slack', 'irc')),
    meeting_id   TEXT,
    target       TEXT NOT NULL,
    msg_suffix   TEXT DEFAULT 'the zoom meeting.',
    include_link BOOLEAN DEFAULT 1,
    enabled      BOOLEAN DEFAULT 1,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS meeting_filters (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    pattern   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS active_meetings (
    meeting_id TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    topic      TEXT,
    host_id    TEXT,
    start_time DATETIME,
    join_url   TEXT
);

CREATE TABLE IF NOT EXISTS participants (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    meeting_id TEXT NOT NULL REFERENCES active_meetings(meeting_id) ON DELETE CASCADE,
    user_name  TEXT,
    email      TEXT,
    join_time  DATETIME,
    leave_time DATETIME
);

CREATE TABLE IF NOT EXISTS irc_configs (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    server    TEXT NOT NULL,
    nick      TEXT NOT NULL,
    password  TEXT NOT NULL,
    use_tls   BOOLEAN DEFAULT 1,
    PRIMARY KEY (tenant_id, server)
);

CREATE TABLE IF NOT EXISTS zoom_credentials (
    tenant_id     TEXT PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    client_id     TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    account_id    TEXT NOT NULL
);

CREATE INDEX idx_subscriptions_tenant ON subscriptions(tenant_id);
CREATE INDEX idx_subscriptions_meeting ON subscriptions(tenant_id, meeting_id);
CREATE INDEX idx_active_meetings_tenant ON active_meetings(tenant_id);
CREATE INDEX idx_participants_meeting ON participants(meeting_id);
