ALTER TABLE users
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS users_tenant_deleted_idx
  ON users (tenant_id, deleted_at)
  WHERE deleted_at IS NULL;
