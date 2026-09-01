CREATE TABLE data_export_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  requested_by UUID NOT NULL,
  export_type TEXT NOT NULL CHECK(export_type IN('FULL','AUDIT','DIRECTORY')),
  reason TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','APPROVED','REJECTED','PROCESSING','READY','FAILED','EXPIRED')),
  reviewed_by UUID,
  review_note TEXT NOT NULL DEFAULT '',
  reviewed_at TIMESTAMPTZ,
  object_key TEXT,
  size_bytes BIGINT,
  sha256 TEXT,
  error_message TEXT,
  expires_at TIMESTAMPTZ,
  downloaded_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(id,tenant_id),
  FOREIGN KEY(requested_by,tenant_id) REFERENCES users(id,tenant_id) ON DELETE RESTRICT,
  FOREIGN KEY(reviewed_by,tenant_id) REFERENCES users(id,tenant_id) ON DELETE RESTRICT,
  CHECK(reviewed_by IS NULL OR reviewed_by<>requested_by),
  CHECK((status='PENDING' AND reviewed_by IS NULL AND reviewed_at IS NULL) OR status<>'PENDING')
);
CREATE UNIQUE INDEX data_export_one_active_request_idx ON data_export_requests(tenant_id,requested_by)
  WHERE status IN('PENDING','APPROVED','PROCESSING');
CREATE INDEX data_export_queue_idx ON data_export_requests(status,created_at) WHERE status='APPROVED';
CREATE INDEX data_export_tenant_created_idx ON data_export_requests(tenant_id,created_at DESC);
