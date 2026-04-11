CREATE INDEX IF NOT EXISTS idx_tenants_zoom_account_id ON tenants(zoom_account_id);
CREATE INDEX IF NOT EXISTS idx_meeting_filters_tenant_id ON meeting_filters(tenant_id);
