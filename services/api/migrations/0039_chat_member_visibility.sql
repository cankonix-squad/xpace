ALTER TABLE chat_members
  ADD COLUMN cleared_at TIMESTAMPTZ,
  ADD COLUMN hidden_at TIMESTAMPTZ;

CREATE INDEX chat_members_visible_idx
  ON chat_members (tenant_id, user_id, hidden_at, joined_at DESC);
