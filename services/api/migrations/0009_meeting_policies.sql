CREATE TABLE tenant_meeting_policies (
  tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
  guest_access_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  waiting_room_default BOOLEAN NOT NULL DEFAULT TRUE,
  recording_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  screen_share_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  updated_by UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT meeting_policy_updater_tenant_fkey
    FOREIGN KEY (updated_by, tenant_id) REFERENCES users(id, tenant_id) ON DELETE SET NULL (updated_by)
);

INSERT INTO tenant_meeting_policies (tenant_id)
SELECT id FROM tenants ON CONFLICT (tenant_id) DO NOTHING;
