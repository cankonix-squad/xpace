ALTER TABLE recordings
  ADD CONSTRAINT recordings_id_tenant_unique UNIQUE (id, tenant_id);

CREATE TABLE recording_access_grants (
  recording_id UUID NOT NULL,
  tenant_id UUID NOT NULL,
  user_id UUID NOT NULL,
  granted_by UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (recording_id, user_id),
  CONSTRAINT recording_access_recording_tenant_fkey
    FOREIGN KEY (recording_id, tenant_id) REFERENCES recordings (id, tenant_id) ON DELETE CASCADE,
  CONSTRAINT recording_access_user_tenant_fkey
    FOREIGN KEY (user_id, tenant_id) REFERENCES users (id, tenant_id) ON DELETE CASCADE,
  CONSTRAINT recording_access_granter_tenant_fkey
    FOREIGN KEY (granted_by, tenant_id) REFERENCES users (id, tenant_id) ON DELETE RESTRICT
);

CREATE INDEX recording_access_user_idx
  ON recording_access_grants (tenant_id, user_id, created_at DESC);
