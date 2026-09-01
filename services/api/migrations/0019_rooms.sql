CREATE TABLE IF NOT EXISTS workspace_rooms (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 2 AND 120),
  description TEXT NOT NULL DEFAULT '',
  created_by UUID NOT NULL,
  conversation_id UUID,
  visibility TEXT NOT NULL DEFAULT 'PRIVATE' CHECK (visibility IN ('PRIVATE','TENANT')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (id, tenant_id),
  FOREIGN KEY (created_by, tenant_id) REFERENCES users(id, tenant_id) ON DELETE RESTRICT,
  FOREIGN KEY (conversation_id, tenant_id) REFERENCES chat_conversations(id, tenant_id) ON DELETE SET NULL (conversation_id)
);
CREATE TABLE IF NOT EXISTS workspace_room_members (
  room_id UUID NOT NULL,
  tenant_id UUID NOT NULL,
  user_id UUID NOT NULL,
  role TEXT NOT NULL DEFAULT 'MEMBER' CHECK (role IN ('OWNER','ADMIN','MEMBER','GUEST')),
  joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (room_id,user_id),
  FOREIGN KEY (room_id, tenant_id) REFERENCES workspace_rooms(id, tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (user_id, tenant_id) REFERENCES users(id, tenant_id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS workspace_room_activity (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  room_id UUID NOT NULL,
  tenant_id UUID NOT NULL,
  actor_id UUID NOT NULL,
  type TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  FOREIGN KEY (room_id, tenant_id) REFERENCES workspace_rooms(id, tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (actor_id, tenant_id) REFERENCES users(id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS workspace_room_members_user_idx ON workspace_room_members(tenant_id,user_id,joined_at DESC);
CREATE INDEX IF NOT EXISTS workspace_room_activity_idx ON workspace_room_activity(tenant_id,room_id,created_at DESC);
