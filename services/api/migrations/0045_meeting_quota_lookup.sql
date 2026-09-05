CREATE INDEX IF NOT EXISTS meetings_tenant_created_idx
  ON meetings (tenant_id, created_at DESC);
