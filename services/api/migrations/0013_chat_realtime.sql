ALTER TABLE chat_members
  ADD COLUMN last_read_at TIMESTAMPTZ,
  ADD COLUMN last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX chat_members_presence_idx ON chat_members (tenant_id, conversation_id, last_seen_at DESC);
