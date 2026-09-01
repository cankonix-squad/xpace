ALTER TABLE drive_nodes ADD COLUMN IF NOT EXISTS room_id UUID;
ALTER TABLE drive_nodes ADD CONSTRAINT drive_nodes_room_tenant_fkey
  FOREIGN KEY (room_id, tenant_id) REFERENCES workspace_rooms(id, tenant_id) ON DELETE SET NULL (room_id);
CREATE INDEX IF NOT EXISTS drive_nodes_room_idx ON drive_nodes(tenant_id,room_id,updated_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS workspace_room_meetings (
  room_id UUID NOT NULL,
  tenant_id UUID NOT NULL,
  meeting_id UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (room_id,meeting_id),
  FOREIGN KEY (room_id,tenant_id) REFERENCES workspace_rooms(id,tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (meeting_id,tenant_id) REFERENCES meetings(id,tenant_id) ON DELETE CASCADE
);
