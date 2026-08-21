CREATE TYPE user_role AS ENUM (
  'SUPER_ADMIN',
  'TENANT_ADMIN',
  'HOST',
  'CO_HOST',
  'MEMBER',
  'GUEST'
);

ALTER TABLE users
  ADD COLUMN role user_role NOT NULL DEFAULT 'MEMBER';

WITH tenant_creators AS (
  SELECT DISTINCT ON (tenant_id) id
  FROM users
  ORDER BY tenant_id, created_at, id
)
UPDATE users SET role = 'TENANT_ADMIN'
WHERE id IN (SELECT id FROM tenant_creators);

UPDATE users SET role = 'SUPER_ADMIN'
WHERE id = (SELECT id FROM users ORDER BY created_at, id LIMIT 1);

ALTER TABLE meeting_participants
  ADD CONSTRAINT meeting_participants_role_check
  CHECK (role IN ('HOST', 'CO_HOST', 'MEMBER', 'GUEST'));

CREATE INDEX users_tenant_role_idx ON users (tenant_id, role);
