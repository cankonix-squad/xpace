CREATE TABLE tenant_governance_policies (
  tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
  recording_retention_days INTEGER NOT NULL DEFAULT 90 CHECK(recording_retention_days BETWEEN 1 AND 3650),
  drive_trash_retention_days INTEGER NOT NULL DEFAULT 30 CHECK(drive_trash_retention_days BETWEEN 1 AND 3650),
  chat_retention_days INTEGER NOT NULL DEFAULT 365 CHECK(chat_retention_days BETWEEN 1 AND 3650),
  audit_retention_days INTEGER NOT NULL DEFAULT 365 CHECK(audit_retention_days BETWEEN 30 AND 3650),
  updated_by UUID,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  FOREIGN KEY(updated_by,tenant_id) REFERENCES users(id,tenant_id) ON DELETE SET NULL (updated_by)
);
INSERT INTO tenant_governance_policies(tenant_id,recording_retention_days)
SELECT t.id,COALESCE(c.recording_retention_days,90) FROM tenants t LEFT JOIN tenant_system_configurations c ON c.tenant_id=t.id ON CONFLICT DO NOTHING;

CREATE TABLE legal_holds (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name TEXT NOT NULL, reason TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('ACTIVE','RELEASED')),
  created_by UUID, released_by UUID, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), released_at TIMESTAMPTZ,
  UNIQUE(id,tenant_id), UNIQUE(tenant_id,name), FOREIGN KEY(created_by,tenant_id) REFERENCES users(id,tenant_id) ON DELETE SET NULL (created_by),
  FOREIGN KEY(released_by,tenant_id) REFERENCES users(id,tenant_id) ON DELETE SET NULL (released_by)
);
CREATE TABLE legal_hold_resources (
  hold_id UUID NOT NULL, tenant_id UUID NOT NULL,
  resource_type TEXT NOT NULL CHECK(resource_type IN('RECORDING','DRIVE_FILE','CHAT_CONVERSATION')),
  resource_id UUID NOT NULL, added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), PRIMARY KEY(hold_id,resource_type,resource_id),
  FOREIGN KEY(hold_id,tenant_id) REFERENCES legal_holds(id,tenant_id) ON DELETE CASCADE
);
CREATE INDEX legal_hold_resources_lookup_idx ON legal_hold_resources(tenant_id,resource_type,resource_id);
ALTER TABLE recordings ADD COLUMN IF NOT EXISTS retention_expired_at TIMESTAMPTZ;
