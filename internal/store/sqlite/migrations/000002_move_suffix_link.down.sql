-- Best-effort down migration.
-- SQLite cannot DROP COLUMN on older versions, so we leave the tenant columns.
-- Recreate meeting_filters without the new columns.

CREATE TABLE meeting_filters_backup (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    pattern   TEXT NOT NULL
);

INSERT INTO meeting_filters_backup (id, tenant_id, pattern)
    SELECT id, tenant_id, pattern FROM meeting_filters;

DROP TABLE meeting_filters;

ALTER TABLE meeting_filters_backup RENAME TO meeting_filters;
