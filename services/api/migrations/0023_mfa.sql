CREATE TABLE IF NOT EXISTS user_mfa (
  user_id UUID NOT NULL,
  tenant_id UUID NOT NULL,
  secret_encrypted TEXT NOT NULL,
  recovery_hashes JSONB NOT NULL DEFAULT '[]'::jsonb,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  confirmed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id),
  FOREIGN KEY (user_id,tenant_id) REFERENCES users(id,tenant_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS user_mfa_tenant_enabled_idx ON user_mfa(tenant_id,enabled);
