CREATE TABLE tenant_scim_configurations (
  tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_by UUID,
  rotated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  FOREIGN KEY (created_by,tenant_id) REFERENCES users(id,tenant_id) ON DELETE SET NULL (created_by)
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS scim_external_id TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS managed_by_scim BOOLEAN NOT NULL DEFAULT FALSE;
CREATE UNIQUE INDEX IF NOT EXISTS users_tenant_scim_external_idx ON users(tenant_id,scim_external_id) WHERE scim_external_id IS NOT NULL;

ALTER TABLE groups ADD COLUMN IF NOT EXISTS scim_external_id TEXT;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS managed_by_scim BOOLEAN NOT NULL DEFAULT FALSE;
CREATE UNIQUE INDEX IF NOT EXISTS groups_tenant_scim_external_idx ON groups(tenant_id,scim_external_id) WHERE scim_external_id IS NOT NULL;
