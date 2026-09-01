ALTER TABLE meeting_participants
  ADD COLUMN external_user_id UUID,
  ADD COLUMN external_tenant_id UUID;

ALTER TABLE meeting_participants
  ADD CONSTRAINT meeting_participants_external_user_tenant_fkey
  FOREIGN KEY (external_user_id, external_tenant_id)
  REFERENCES users (id, tenant_id)
  ON DELETE SET NULL (external_user_id, external_tenant_id);

ALTER TABLE meeting_participants
  ADD CONSTRAINT meeting_participants_identity_scope_check
  CHECK (
    (external_user_id IS NULL AND external_tenant_id IS NULL)
    OR
    (user_id IS NULL AND external_user_id IS NOT NULL AND external_tenant_id IS NOT NULL)
  );

CREATE UNIQUE INDEX meeting_participants_active_external_user_unique
  ON meeting_participants (meeting_id, external_user_id)
  WHERE external_user_id IS NOT NULL
    AND status NOT IN ('LEFT', 'REMOVED');

CREATE INDEX meeting_participants_external_identity_idx
  ON meeting_participants (external_tenant_id, external_user_id, created_at DESC)
  WHERE external_user_id IS NOT NULL;
