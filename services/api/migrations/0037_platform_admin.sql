ALTER TABLE tenants
  ADD COLUMN platform_status TEXT NOT NULL DEFAULT 'ACTIVE'
    CHECK (platform_status IN ('ACTIVE','SUSPENDED')),
  ADD COLUMN suspended_at TIMESTAMPTZ,
  ADD COLUMN suspended_by UUID REFERENCES users(id) ON DELETE SET NULL,
  ADD COLUMN suspension_reason TEXT NOT NULL DEFAULT '';

CREATE INDEX tenants_platform_status_idx ON tenants(platform_status,created_at DESC);

CREATE TABLE platform_support_access (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  actor_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  reason TEXT NOT NULL CHECK (char_length(reason) BETWEEN 5 AND 500),
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CHECK (expires_at > created_at)
);

CREATE INDEX platform_support_access_lookup_idx
  ON platform_support_access(actor_user_id,tenant_id,expires_at DESC)
  WHERE revoked_at IS NULL;
