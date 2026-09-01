CREATE TABLE IF NOT EXISTS calendar_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  organizer_id UUID NOT NULL,
  meeting_id UUID,
  title TEXT NOT NULL CHECK (length(btrim(title)) BETWEEN 3 AND 160),
  description TEXT NOT NULL DEFAULT '',
  timezone TEXT NOT NULL DEFAULT 'Asia/Jakarta',
  starts_at TIMESTAMPTZ NOT NULL,
  ends_at TIMESTAMPTZ NOT NULL,
  recurrence_rule TEXT,
  reminder_minutes INTEGER NOT NULL DEFAULT 15 CHECK (reminder_minutes BETWEEN 0 AND 10080),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (id, tenant_id),
  CHECK (ends_at > starts_at),
  FOREIGN KEY (organizer_id, tenant_id) REFERENCES users(id, tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (meeting_id, tenant_id) REFERENCES meetings(id, tenant_id) ON DELETE SET NULL (meeting_id)
);

CREATE TABLE IF NOT EXISTS calendar_event_attendees (
  event_id UUID NOT NULL,
  tenant_id UUID NOT NULL,
  user_id UUID NOT NULL,
  status TEXT NOT NULL DEFAULT 'INVITED' CHECK (status IN ('INVITED','ACCEPTED','DECLINED')),
  responded_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (event_id, user_id),
  FOREIGN KEY (event_id, tenant_id) REFERENCES calendar_events(id, tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (user_id, tenant_id) REFERENCES users(id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS calendar_events_tenant_start_idx ON calendar_events(tenant_id, starts_at);
CREATE INDEX IF NOT EXISTS calendar_attendees_user_idx ON calendar_event_attendees(tenant_id, user_id, status);
