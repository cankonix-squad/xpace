CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE user_status AS ENUM ('ACTIVE', 'INVITED', 'SUSPENDED', 'DEACTIVATED');
CREATE TYPE meeting_status AS ENUM ('SCHEDULED', 'WAITING', 'ACTIVE', 'ENDED', 'CANCELLED');
CREATE TYPE participant_status AS ENUM ('PRE_JOIN', 'WAITING_ROOM', 'JOINED', 'LEFT', 'REMOVED', 'DISCONNECTED');

CREATE TABLE tenants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
  email TEXT NOT NULL,
  username TEXT NOT NULL,
  display_name TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  status user_status NOT NULL DEFAULT 'ACTIVE',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, email),
  UNIQUE (tenant_id, username)
);

CREATE TABLE meetings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
  host_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  room_name TEXT NOT NULL UNIQUE,
  join_code TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  scheduled_at TIMESTAMPTZ,
  status meeting_status NOT NULL DEFAULT 'SCHEDULED',
  waiting_room_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  locked_at TIMESTAMPTZ,
  started_at TIMESTAMPTZ,
  ended_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE meeting_participants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  meeting_id UUID NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  display_name TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'MEMBER',
  status participant_status NOT NULL DEFAULT 'PRE_JOIN',
  joined_at TIMESTAMPTZ,
  left_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX meetings_tenant_status_idx ON meetings (tenant_id, status);
CREATE INDEX meeting_participants_meeting_idx ON meeting_participants (meeting_id, status);
