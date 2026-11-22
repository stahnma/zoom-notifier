-- Reverts to per-tenant Zoom credentials and meeting state. Per-account webhook
-- secrets have no equivalent in the old schema and are discarded; accounts
-- shared by several tenants copy their credentials to each tenant, and their
-- meetings are assigned to one of those tenants.

CREATE TABLE zoom_credentials (
    tenant_id     TEXT PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    client_id     TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    account_id    TEXT NOT NULL
);

INSERT INTO zoom_credentials (tenant_id, client_id, client_secret, account_id)
SELECT t.id, za.client_id, za.client_secret, za.account_id
FROM tenants t
JOIN zoom_accounts za ON za.account_id = t.zoom_account_id
WHERE za.client_id != '' AND za.client_secret != '';

CREATE TABLE active_meetings_old (
    meeting_id TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    topic      TEXT,
    host_id    TEXT,
    start_time DATETIME,
    join_url   TEXT
);

CREATE TABLE participants_old (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    meeting_id TEXT NOT NULL REFERENCES active_meetings_old(meeting_id) ON DELETE CASCADE,
    user_name  TEXT,
    email      TEXT,
    join_time  DATETIME,
    leave_time DATETIME
);

INSERT OR IGNORE INTO active_meetings_old (meeting_id, tenant_id, topic, host_id, start_time, join_url)
SELECT m.meeting_id,
       (SELECT MIN(t.id) FROM tenants t WHERE t.zoom_account_id = m.zoom_account_id),
       m.topic, m.host_id, m.start_time, m.join_url
FROM active_meetings m
WHERE EXISTS (SELECT 1 FROM tenants t WHERE t.zoom_account_id = m.zoom_account_id);

INSERT INTO participants_old (meeting_id, user_name, email, join_time, leave_time)
SELECT p.meeting_id, p.user_name, p.email, p.join_time, p.leave_time
FROM participants p
WHERE p.meeting_id IN (SELECT meeting_id FROM active_meetings_old)
ORDER BY p.id;

DROP TABLE participants;
DROP TABLE active_meetings;
DROP TABLE zoom_accounts;
ALTER TABLE active_meetings_old RENAME TO active_meetings;
ALTER TABLE participants_old RENAME TO participants;

CREATE INDEX idx_active_meetings_tenant ON active_meetings(tenant_id);
CREATE INDEX idx_participants_meeting ON participants(meeting_id);
