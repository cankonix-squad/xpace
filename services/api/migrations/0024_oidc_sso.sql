CREATE TABLE tenant_oidc_configurations (
  tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
  issuer_url TEXT NOT NULL,
  authorization_endpoint TEXT NOT NULL,
  token_endpoint TEXT NOT NULL,
  userinfo_endpoint TEXT NOT NULL,
  client_id TEXT NOT NULL,
  client_secret_encrypted TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  auto_provision BOOLEAN NOT NULL DEFAULT FALSE,
  default_role TEXT NOT NULL DEFAULT 'MEMBER' CHECK (default_role IN ('MEMBER','GUEST')),
  updated_by UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  FOREIGN KEY (updated_by,tenant_id) REFERENCES users(id,tenant_id) ON DELETE SET NULL (updated_by)
);

CREATE TABLE oidc_login_states (
  state_hash TEXT PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  nonce_hash TEXT NOT NULL,
  verifier_encrypted TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX oidc_login_states_expiry_idx ON oidc_login_states(expires_at);

CREATE TABLE user_oidc_identities (
  user_id UUID NOT NULL,
  tenant_id UUID NOT NULL,
  issuer_url TEXT NOT NULL,
  subject TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (tenant_id,issuer_url,subject),
  UNIQUE (user_id,issuer_url),
  FOREIGN KEY (user_id,tenant_id) REFERENCES users(id,tenant_id) ON DELETE CASCADE
);
