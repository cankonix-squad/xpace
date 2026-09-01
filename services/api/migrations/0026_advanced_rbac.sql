CREATE TABLE custom_roles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_by UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(tenant_id,name),
  FOREIGN KEY (created_by,tenant_id) REFERENCES users(id,tenant_id) ON DELETE SET NULL (created_by)
);

CREATE TABLE custom_role_permissions (
  role_id UUID NOT NULL REFERENCES custom_roles(id) ON DELETE CASCADE,
  permission TEXT NOT NULL,
  PRIMARY KEY(role_id,permission)
);

CREATE TABLE user_custom_roles (
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  role_id UUID NOT NULL REFERENCES custom_roles(id) ON DELETE CASCADE,
  assigned_by UUID,
  assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY(user_id,role_id),
  FOREIGN KEY (user_id,tenant_id) REFERENCES users(id,tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (assigned_by,tenant_id) REFERENCES users(id,tenant_id) ON DELETE SET NULL (assigned_by)
);
CREATE INDEX user_custom_roles_tenant_user_idx ON user_custom_roles(tenant_id,user_id);
