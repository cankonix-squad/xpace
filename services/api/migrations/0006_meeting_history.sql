CREATE INDEX meetings_tenant_history_idx
  ON meetings (tenant_id, ended_at DESC, id)
  WHERE status IN ('ENDED', 'CANCELLED');

CREATE INDEX meeting_participants_user_history_idx
  ON meeting_participants (tenant_id, user_id, meeting_id, created_at DESC)
  WHERE user_id IS NOT NULL;
