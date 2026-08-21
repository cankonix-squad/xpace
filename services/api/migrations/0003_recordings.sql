CREATE TYPE recording_status AS ENUM ('STARTING', 'RECORDING', 'STOPPING', 'READY', 'FAILED');

CREATE TABLE recordings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
  meeting_id UUID NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
  started_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  egress_id TEXT UNIQUE,
  object_key TEXT NOT NULL UNIQUE,
  status recording_status NOT NULL DEFAULT 'STARTING',
  started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  stopped_at TIMESTAMPTZ,
  duration_seconds INTEGER,
  size_bytes BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX recordings_one_active_per_meeting_idx
  ON recordings (meeting_id) WHERE status IN ('STARTING', 'RECORDING', 'STOPPING');
CREATE INDEX recordings_tenant_created_idx ON recordings (tenant_id, created_at DESC);
