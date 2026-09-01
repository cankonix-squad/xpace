ALTER TABLE incidents ADD COLUMN IF NOT EXISTS external_key TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS incidents_external_source_key_idx
  ON incidents(tenant_id,source,external_key)
  WHERE external_key IS NOT NULL;

ALTER TABLE incident_events DROP CONSTRAINT IF EXISTS incident_events_event_type_check;
ALTER TABLE incident_events ADD CONSTRAINT incident_events_event_type_check
  CHECK (event_type IN ('CREATED','ACKNOWLEDGED','INVESTIGATING','ASSIGNED','SEVERITY_CHANGED','NOTE','RESOLVED','CLOSED','REOPENED','ALERT_FIRING','ALERT_RESOLVED'));
