CREATE TABLE IF NOT EXISTS drive_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  node_id UUID NOT NULL,
  version INTEGER NOT NULL CHECK (version > 0),
  object_key TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
  created_by UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (node_id, version),
  FOREIGN KEY (node_id, tenant_id) REFERENCES drive_nodes(id, tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (created_by, tenant_id) REFERENCES users(id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS drive_versions_node_idx ON drive_versions(tenant_id,node_id,version DESC);
