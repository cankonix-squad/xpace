CREATE TABLE IF NOT EXISTS incidents (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  title TEXT NOT NULL CHECK (char_length(title) BETWEEN 3 AND 160),
  description TEXT NOT NULL DEFAULT '' CHECK (char_length(description) <= 4000),
  source TEXT NOT NULL CHECK (source IN ('MANUAL','PROMETHEUS','CLIENT_ERROR')),
  severity TEXT NOT NULL CHECK (severity IN ('P1','P2','P3','P4')),
  status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','ACKNOWLEDGED','INVESTIGATING','RESOLVED','CLOSED')),
  assignee_user_id UUID,
  created_by UUID,
  acknowledged_by UUID,
  acknowledged_at TIMESTAMPTZ,
  resolved_by UUID,
  resolved_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id,id),
  FOREIGN KEY (tenant_id,assignee_user_id) REFERENCES users(tenant_id,id) ON DELETE SET NULL,
  FOREIGN KEY (tenant_id,created_by) REFERENCES users(tenant_id,id) ON DELETE SET NULL,
  FOREIGN KEY (tenant_id,acknowledged_by) REFERENCES users(tenant_id,id) ON DELETE SET NULL,
  FOREIGN KEY (tenant_id,resolved_by) REFERENCES users(tenant_id,id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS incidents_tenant_status_updated_idx ON incidents(tenant_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS incident_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  incident_id UUID NOT NULL,
  actor_user_id UUID,
  event_type TEXT NOT NULL CHECK (event_type IN ('CREATED','ACKNOWLEDGED','INVESTIGATING','ASSIGNED','SEVERITY_CHANGED','NOTE','RESOLVED','CLOSED','REOPENED')),
  note TEXT NOT NULL DEFAULT '' CHECK (char_length(note) <= 4000),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  FOREIGN KEY (tenant_id,incident_id) REFERENCES incidents(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,actor_user_id) REFERENCES users(tenant_id,id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS incident_events_incident_created_idx ON incident_events(tenant_id,incident_id,created_at,id);
