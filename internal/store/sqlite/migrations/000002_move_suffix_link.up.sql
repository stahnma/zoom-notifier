-- Add tenant-wide defaults for msg_suffix and include_link
ALTER TABLE tenants ADD COLUMN default_msg_suffix TEXT DEFAULT 'the zoom meeting.';
ALTER TABLE tenants ADD COLUMN default_include_link BOOLEAN DEFAULT 1;

-- Add per-filter overrides (nullable)
ALTER TABLE meeting_filters ADD COLUMN msg_suffix TEXT;
ALTER TABLE meeting_filters ADD COLUMN include_link BOOLEAN;

-- Data migration: copy the first subscription's values to the tenant defaults
UPDATE tenants SET
    default_msg_suffix = COALESCE(
        (SELECT s.msg_suffix FROM subscriptions s WHERE s.tenant_id = tenants.id ORDER BY s.id LIMIT 1),
        'the zoom meeting.'
    ),
    default_include_link = COALESCE(
        (SELECT s.include_link FROM subscriptions s WHERE s.tenant_id = tenants.id ORDER BY s.id LIMIT 1),
        1
    );
