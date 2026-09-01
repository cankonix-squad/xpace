ALTER TABLE chat_messages
  ADD CONSTRAINT chat_messages_id_tenant_unique UNIQUE (id, tenant_id),
  ADD COLUMN parent_message_id UUID,
  ADD COLUMN edited_at TIMESTAMPTZ,
  ADD COLUMN deleted_at TIMESTAMPTZ,
  ADD COLUMN pinned_at TIMESTAMPTZ,
  ADD CONSTRAINT chat_messages_parent_fk FOREIGN KEY (parent_message_id) REFERENCES chat_messages(id) ON DELETE SET NULL;

CREATE TABLE chat_reactions (
  message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
  tenant_id UUID NOT NULL,
  user_id UUID NOT NULL,
  emoji TEXT NOT NULL CHECK (emoji IN ('👍','❤️','😂','🎉','😮','😢')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (message_id, user_id, emoji),
  FOREIGN KEY (message_id, tenant_id) REFERENCES chat_messages(id, tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (user_id, tenant_id) REFERENCES users(id, tenant_id) ON DELETE CASCADE
);

CREATE INDEX chat_reactions_message_idx ON chat_reactions (tenant_id, message_id, created_at DESC);
