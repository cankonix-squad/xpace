CREATE TABLE IF NOT EXISTS chat_attachments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  conversation_id UUID NOT NULL,
  message_id UUID NOT NULL,
  uploader_id UUID NOT NULL,
  object_key TEXT NOT NULL,
  original_name TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size_bytes BIGINT NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 26214400),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  FOREIGN KEY (tenant_id, conversation_id) REFERENCES chat_conversations(tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, message_id) REFERENCES chat_messages(tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, uploader_id) REFERENCES users(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS chat_attachments_message_idx ON chat_attachments(tenant_id, message_id, created_at DESC);
