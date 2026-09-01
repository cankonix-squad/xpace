CREATE TABLE notifications (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  recipient_id UUID NOT NULL,
  actor_id UUID,
  type TEXT NOT NULL CHECK (type IN ('CHAT_REPLY','CHAT_REACTION','CHAT_MENTION')),
  conversation_id UUID,
  message_id UUID,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  read_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  FOREIGN KEY (recipient_id, tenant_id) REFERENCES users(id, tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (actor_id, tenant_id) REFERENCES users(id, tenant_id) ON DELETE SET NULL,
  FOREIGN KEY (conversation_id, tenant_id) REFERENCES chat_conversations(id, tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (message_id, tenant_id) REFERENCES chat_messages(id, tenant_id) ON DELETE CASCADE
);

CREATE INDEX notifications_recipient_idx ON notifications (tenant_id, recipient_id, read_at, created_at DESC);
