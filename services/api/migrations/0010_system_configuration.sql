CREATE TABLE tenant_system_configurations (
  tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
  default_timezone TEXT NOT NULL DEFAULT 'Asia/Jakarta',
  default_locale TEXT NOT NULL DEFAULT 'id-ID',
  support_email TEXT NOT NULL DEFAULT '',
  max_meeting_duration_minutes INTEGER NOT NULL DEFAULT 120 CHECK (max_meeting_duration_minutes BETWEEN 15 AND 1440),
  recording_retention_days INTEGER NOT NULL DEFAULT 30 CHECK (recording_retention_days BETWEEN 1 AND 3650),
  updated_by UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT system_configuration_updater_tenant_fkey
    FOREIGN KEY (updated_by, tenant_id) REFERENCES users(id, tenant_id) ON DELETE SET NULL (updated_by)
);

INSERT INTO tenant_system_configurations (tenant_id)
SELECT id FROM tenants ON CONFLICT (tenant_id) DO NOTHING;
