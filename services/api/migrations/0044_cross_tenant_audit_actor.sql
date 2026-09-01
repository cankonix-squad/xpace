ALTER TABLE audit_events ADD COLUMN actor_tenant_id UUID;

UPDATE audit_events
SET actor_tenant_id = tenant_id
WHERE actor_user_id IS NOT NULL;

ALTER TABLE audit_events DROP CONSTRAINT audit_events_actor_tenant_fkey;

ALTER TABLE audit_events
  ADD CONSTRAINT audit_events_actor_identity_fkey
  FOREIGN KEY (actor_user_id, actor_tenant_id)
  REFERENCES users (id, tenant_id)
  ON DELETE SET NULL (actor_user_id, actor_tenant_id),
  ADD CONSTRAINT audit_events_actor_identity_complete
  CHECK ((actor_user_id IS NULL) = (actor_tenant_id IS NULL));

CREATE INDEX audit_events_actor_identity_idx
  ON audit_events (actor_tenant_id, actor_user_id, created_at DESC)
  WHERE actor_user_id IS NOT NULL;
