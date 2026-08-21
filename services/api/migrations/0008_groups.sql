CREATE TABLE groups (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_by UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, name),
  UNIQUE (id, tenant_id),
  CONSTRAINT groups_creator_tenant_fkey
    FOREIGN KEY (created_by, tenant_id) REFERENCES users(id, tenant_id) ON DELETE RESTRICT
);

CREATE TABLE group_members (
  group_id UUID NOT NULL,
  tenant_id UUID NOT NULL,
  user_id UUID NOT NULL,
  added_by UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (group_id, user_id),
  CONSTRAINT group_members_group_tenant_fkey
    FOREIGN KEY (group_id, tenant_id) REFERENCES groups(id, tenant_id) ON DELETE CASCADE,
  CONSTRAINT group_members_user_tenant_fkey
    FOREIGN KEY (user_id, tenant_id) REFERENCES users(id, tenant_id) ON DELETE CASCADE,
  CONSTRAINT group_members_adder_tenant_fkey
    FOREIGN KEY (added_by, tenant_id) REFERENCES users(id, tenant_id) ON DELETE RESTRICT
);

CREATE INDEX group_members_tenant_user_idx ON group_members (tenant_id, user_id);
