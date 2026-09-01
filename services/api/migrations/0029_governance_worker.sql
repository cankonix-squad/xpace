ALTER TABLE recordings ADD COLUMN IF NOT EXISTS storage_deleted_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS recordings_retention_purge_idx
  ON recordings(tenant_id,retention_expired_at)
  WHERE retention_expired_at IS NOT NULL AND storage_deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS chat_attachments_retention_idx
  ON chat_attachments(tenant_id,conversation_id,created_at);
