CREATE TABLE IF NOT EXISTS error_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  user_id UUID NOT NULL,
  source TEXT NOT NULL CHECK (source IN ('WEB')),
  message TEXT NOT NULL,
  digest TEXT,
  path TEXT,
  release TEXT NOT NULL,
  user_agent TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  FOREIGN KEY (tenant_id, user_id) REFERENCES users(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS error_events_tenant_created_idx ON error_events(tenant_id, created_at DESC);
