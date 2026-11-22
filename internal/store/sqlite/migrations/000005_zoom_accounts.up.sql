-- Zoom accounts become first-class: one row per Zoom account holds its
-- webhook secret and Server-to-Server OAuth credentials, and many tenants may
-- link to it via tenants.zoom_account_id. Meeting state moves from per-tenant
-- to per-account so every tenant on an account sees the same meetings.
--
-- Secret columns are copied verbatim: the cipher binds no associated data, so
-- enc:v1: ciphertexts remain valid in their new table.

CREATE TABLE zoom_accounts (
    account_id     TEXT PRIMARY KEY,
    webhook_secret TEXT NOT NULL DEFAULT '',
    client_id      TEXT NOT NULL DEFAULT '',
    client_secret  TEXT NOT NULL DEFAULT '',
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Tenants that only set credentials via the API never had their routing
-- account ID set; link them to the account their credentials belong to.
UPDATE tenants SET zoom_account_id = (
    SELECT zc.account_id FROM zoom_credentials zc WHERE zc.tenant_id = tenants.id
)
WHERE (zoom_account_id IS NULL OR zoom_account_id = '')
  AND EXISTS (SELECT 1 FROM zoom_credentials zc WHERE zc.tenant_id = tenants.id AND zc.account_id != '');

-- Credentials move to their account. If several tenants stored credentials for
-- the same account, the most recently created tenant's copy wins.
INSERT OR IGNORE INTO zoom_accounts (account_id, client_id, client_secret)
SELECT zc.account_id, zc.client_id, zc.client_secret
FROM zoom_credentials zc
JOIN tenants t ON t.id = zc.tenant_id
WHERE zc.account_id != ''
ORDER BY t.installed_at DESC, t.id DESC;

-- Every linked account gets a row (without a webhook secret, it falls back to
-- the global zoom.webhook_secret).
INSERT OR IGNORE INTO zoom_accounts (account_id)
SELECT DISTINCT zoom_account_id FROM tenants
WHERE zoom_account_id IS NOT NULL AND zoom_account_id != '';

DROP TABLE zoom_credentials;

-- Meeting state keyed by (zoom_account_id, meeting_id).
CREATE TABLE active_meetings_new (
    zoom_account_id TEXT NOT NULL REFERENCES zoom_accounts(account_id) ON DELETE CASCADE,
    meeting_id      TEXT NOT NULL,
    topic           TEXT,
    host_id         TEXT,
    start_time      DATETIME,
    join_url        TEXT,
    PRIMARY KEY (zoom_account_id, meeting_id)
);

CREATE TABLE participants_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    zoom_account_id TEXT NOT NULL,
    meeting_id      TEXT NOT NULL,
    user_name       TEXT,
    email           TEXT,
    join_time       DATETIME,
    leave_time      DATETIME,
    FOREIGN KEY (zoom_account_id, meeting_id)
        REFERENCES active_meetings_new(zoom_account_id, meeting_id) ON DELETE CASCADE
);

INSERT OR IGNORE INTO active_meetings_new (zoom_account_id, meeting_id, topic, host_id, start_time, join_url)
SELECT t.zoom_account_id, m.meeting_id, m.topic, m.host_id, m.start_time, m.join_url
FROM active_meetings m
JOIN tenants t ON t.id = m.tenant_id
WHERE t.zoom_account_id IS NOT NULL AND t.zoom_account_id != '';

-- Collapse the per-tenant duplicate rows the old fan-out produced.
INSERT INTO participants_new (zoom_account_id, meeting_id, user_name, email, join_time, leave_time)
SELECT am.zoom_account_id, p.meeting_id, p.user_name, p.email, p.join_time, MAX(p.leave_time)
FROM participants p
JOIN active_meetings_new am ON am.meeting_id = p.meeting_id
GROUP BY am.zoom_account_id, p.meeting_id, p.user_name, p.email, p.join_time
ORDER BY MIN(p.id);

DROP TABLE participants;
DROP TABLE active_meetings;
ALTER TABLE active_meetings_new RENAME TO active_meetings;
ALTER TABLE participants_new RENAME TO participants;

CREATE INDEX idx_participants_meeting ON participants(zoom_account_id, meeting_id);
