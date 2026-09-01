CREATE TABLE IF NOT EXISTS drive_nodes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  parent_id UUID,
  owner_id UUID NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('FILE','FOLDER')),
  name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 255),
  object_key TEXT,
  content_type TEXT,
  size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  retention_until TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (id, tenant_id),
  FOREIGN KEY (parent_id, tenant_id) REFERENCES drive_nodes(id, tenant_id) ON DELETE RESTRICT,
  FOREIGN KEY (owner_id, tenant_id) REFERENCES users(id, tenant_id) ON DELETE CASCADE,
  CHECK ((kind='FOLDER' AND object_key IS NULL) OR (kind='FILE' AND object_key IS NOT NULL))
);
CREATE TABLE IF NOT EXISTS drive_shares (
  node_id UUID NOT NULL,
  tenant_id UUID NOT NULL,
  user_id UUID NOT NULL,
  permission TEXT NOT NULL CHECK (permission IN ('VIEW','EDIT')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (node_id,user_id),
  FOREIGN KEY (node_id,tenant_id) REFERENCES drive_nodes(id,tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (user_id,tenant_id) REFERENCES users(id,tenant_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS drive_nodes_parent_idx ON drive_nodes(tenant_id,parent_id,updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS drive_shares_user_idx ON drive_shares(tenant_id,user_id);
