ALTER TABLE meetings
  ADD COLUMN end_reason TEXT,
  ADD COLUMN room_closed_at TIMESTAMPTZ;

CREATE INDEX meetings_duration_enforcement_idx
  ON meetings (status, started_at)
  WHERE status = 'ACTIVE' OR (status = 'ENDED' AND room_closed_at IS NULL);
