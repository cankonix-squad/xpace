ALTER TABLE users ADD CONSTRAINT users_id_tenant_unique UNIQUE (id, tenant_id);
ALTER TABLE meetings ADD CONSTRAINT meetings_id_tenant_unique UNIQUE (id, tenant_id);

ALTER TABLE meetings DROP CONSTRAINT meetings_host_id_fkey;
ALTER TABLE meetings ADD CONSTRAINT meetings_host_tenant_fkey
  FOREIGN KEY (host_id, tenant_id) REFERENCES users (id, tenant_id) ON DELETE RESTRICT;

ALTER TABLE meeting_participants ADD COLUMN tenant_id UUID;
UPDATE meeting_participants AS participant
SET tenant_id = meeting.tenant_id
FROM meetings AS meeting
WHERE meeting.id = participant.meeting_id;
ALTER TABLE meeting_participants ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE meeting_participants DROP CONSTRAINT meeting_participants_meeting_id_fkey;
ALTER TABLE meeting_participants DROP CONSTRAINT meeting_participants_user_id_fkey;
ALTER TABLE meeting_participants ADD CONSTRAINT meeting_participants_meeting_tenant_fkey
  FOREIGN KEY (meeting_id, tenant_id) REFERENCES meetings (id, tenant_id) ON DELETE CASCADE;
ALTER TABLE meeting_participants ADD CONSTRAINT meeting_participants_user_tenant_fkey
  FOREIGN KEY (user_id, tenant_id) REFERENCES users (id, tenant_id) ON DELETE SET NULL (user_id);
CREATE INDEX meeting_participants_tenant_status_idx
  ON meeting_participants (tenant_id, status);

ALTER TABLE recordings DROP CONSTRAINT recordings_meeting_id_fkey;
ALTER TABLE recordings DROP CONSTRAINT recordings_started_by_fkey;
ALTER TABLE recordings ADD CONSTRAINT recordings_meeting_tenant_fkey
  FOREIGN KEY (meeting_id, tenant_id) REFERENCES meetings (id, tenant_id) ON DELETE CASCADE;
ALTER TABLE recordings ADD CONSTRAINT recordings_started_by_tenant_fkey
  FOREIGN KEY (started_by, tenant_id) REFERENCES users (id, tenant_id) ON DELETE RESTRICT;

ALTER TABLE audit_events DROP CONSTRAINT audit_events_actor_user_id_fkey;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_actor_tenant_fkey
  FOREIGN KEY (actor_user_id, tenant_id) REFERENCES users (id, tenant_id) ON DELETE SET NULL (actor_user_id);

CREATE TABLE user_profiles (
  user_id UUID NOT NULL,
  tenant_id UUID NOT NULL,
  timezone TEXT NOT NULL DEFAULT 'Asia/Jakarta',
  locale TEXT NOT NULL DEFAULT 'en-ID',
  bio TEXT NOT NULL DEFAULT '',
  avatar_url TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id),
  CONSTRAINT user_profiles_user_tenant_fkey
    FOREIGN KEY (user_id, tenant_id) REFERENCES users (id, tenant_id) ON DELETE CASCADE
);

INSERT INTO user_profiles (user_id, tenant_id)
SELECT id, tenant_id FROM users
ON CONFLICT (user_id) DO NOTHING;

CREATE INDEX user_profiles_tenant_idx ON user_profiles (tenant_id);
