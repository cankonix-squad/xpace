CREATE TYPE chat_conversation_type AS ENUM ('DIRECT', 'CHANNEL');

CREATE TABLE chat_conversations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  type chat_conversation_type NOT NULL,
  name TEXT,
  created_by UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (id, tenant_id),
  FOREIGN KEY (created_by, tenant_id) REFERENCES users(id, tenant_id) ON DELETE RESTRICT,
  CHECK ((type = 'CHANNEL' AND name IS NOT NULL AND length(btrim(name)) BETWEEN 2 AND 120) OR type = 'DIRECT')
);

CREATE TABLE chat_members (
  conversation_id UUID NOT NULL,
  tenant_id UUID NOT NULL,
  user_id UUID NOT NULL,
  joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (conversation_id, user_id),
  FOREIGN KEY (conversation_id, tenant_id) REFERENCES chat_conversations(id, tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (user_id, tenant_id) REFERENCES users(id, tenant_id) ON DELETE CASCADE
);

CREATE TABLE chat_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  conversation_id UUID NOT NULL,
  sender_id UUID NOT NULL,
  body TEXT NOT NULL CHECK (length(btrim(body)) BETWEEN 1 AND 4000),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  FOREIGN KEY (conversation_id, tenant_id) REFERENCES chat_conversations(id, tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (sender_id, tenant_id) REFERENCES users(id, tenant_id) ON DELETE RESTRICT
);

CREATE INDEX chat_members_user_idx ON chat_members (tenant_id, user_id, joined_at DESC);
CREATE INDEX chat_messages_conversation_idx ON chat_messages (tenant_id, conversation_id, created_at DESC, id DESC);
