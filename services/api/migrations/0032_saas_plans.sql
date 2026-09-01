CREATE TABLE saas_plans (
  key TEXT PRIMARY KEY CHECK (key ~ '^[A-Z][A-Z0-9_]*$'),
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  price_monthly_idr BIGINT NOT NULL CHECK (price_monthly_idr >= 0),
  trial_days INTEGER NOT NULL DEFAULT 0 CHECK (trial_days BETWEEN 0 AND 90),
  max_users BIGINT NOT NULL CHECK (max_users > 0),
  max_meetings_per_month BIGINT NOT NULL CHECK (max_meetings_per_month > 0),
  max_meeting_duration_minutes BIGINT NOT NULL CHECK (max_meeting_duration_minutes > 0),
  max_storage_bytes BIGINT NOT NULL CHECK (max_storage_bytes > 0),
  max_recordings_per_month BIGINT NOT NULL CHECK (max_recordings_per_month >= 0),
  features JSONB NOT NULL DEFAULT '{}'::jsonb,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO saas_plans(key,name,description,price_monthly_idr,trial_days,max_users,max_meetings_per_month,max_meeting_duration_minutes,max_storage_bytes,max_recordings_per_month,features,sort_order) VALUES
('STARTER','Starter','For small teams getting started with secure collaboration.',149000,14,5,100,60,5368709120,10,'{"recording":true,"drive":true,"chatAttachments":true,"advancedGovernance":false,"sso":false}'::jsonb,10),
('BUSINESS','Business','For growing teams that need administration and governance.',499000,14,25,1000,240,107374182400,500,'{"recording":true,"drive":true,"chatAttachments":true,"advancedGovernance":true,"sso":true}'::jsonb,20),
('ENTERPRISE','Enterprise','Custom scale, identity, governance, and support.',0,30,1000000,1000000,1440,10995116277760,1000000,'{"recording":true,"drive":true,"chatAttachments":true,"advancedGovernance":true,"sso":true}'::jsonb,30);

CREATE TABLE tenant_subscriptions (
  tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
  plan_key TEXT NOT NULL REFERENCES saas_plans(key) ON DELETE RESTRICT,
  status TEXT NOT NULL CHECK (status IN ('TRIALING','ACTIVE','PAST_DUE','CANCELED','SUSPENDED')),
  trial_started_at TIMESTAMPTZ,
  trial_ends_at TIMESTAMPTZ,
  current_period_started_at TIMESTAMPTZ,
  current_period_ends_at TIMESTAMPTZ,
  cancel_at_period_end BOOLEAN NOT NULL DEFAULT FALSE,
  billing_customer_id TEXT,
  billing_subscription_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CHECK ((status <> 'TRIALING') OR (trial_started_at IS NOT NULL AND trial_ends_at IS NOT NULL))
);

CREATE UNIQUE INDEX tenant_subscriptions_billing_customer_idx ON tenant_subscriptions(billing_customer_id) WHERE billing_customer_id IS NOT NULL;
CREATE UNIQUE INDEX tenant_subscriptions_billing_subscription_idx ON tenant_subscriptions(billing_subscription_id) WHERE billing_subscription_id IS NOT NULL;
CREATE INDEX tenant_subscriptions_status_idx ON tenant_subscriptions(status,trial_ends_at,current_period_ends_at);

CREATE TABLE tenant_entitlements (
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  entitlement_key TEXT NOT NULL,
  enabled BOOLEAN,
  limit_value BIGINT,
  reason TEXT NOT NULL DEFAULT '',
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY(tenant_id,entitlement_key),
  CHECK (enabled IS NOT NULL OR limit_value IS NOT NULL)
);

INSERT INTO tenant_subscriptions(tenant_id,plan_key,status,current_period_started_at,current_period_ends_at)
SELECT id,'BUSINESS','ACTIVE',NOW(),NOW()+INTERVAL '100 years' FROM tenants
ON CONFLICT(tenant_id) DO NOTHING;
