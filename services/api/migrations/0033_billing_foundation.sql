CREATE TABLE billing_checkout_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  requested_by UUID REFERENCES users(id) ON DELETE SET NULL,
  plan_key TEXT NOT NULL REFERENCES saas_plans(key) ON DELETE RESTRICT,
  provider TEXT NOT NULL,
  provider_session_id TEXT,
  status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','OPEN','COMPLETED','EXPIRED','CANCELED')),
  checkout_url TEXT,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX billing_checkout_sessions_provider_idx ON billing_checkout_sessions(provider,provider_session_id) WHERE provider_session_id IS NOT NULL;
CREATE INDEX billing_checkout_sessions_tenant_idx ON billing_checkout_sessions(tenant_id,created_at DESC);

CREATE TABLE billing_invoices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  provider_invoice_id TEXT NOT NULL,
  invoice_number TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status IN ('DRAFT','PENDING','PAID','FAILED','VOID','REFUNDED')),
  currency TEXT NOT NULL DEFAULT 'IDR' CHECK (currency ~ '^[A-Z]{3}$'),
  subtotal_amount BIGINT NOT NULL DEFAULT 0 CHECK (subtotal_amount >= 0),
  tax_amount BIGINT NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
  total_amount BIGINT NOT NULL DEFAULT 0 CHECK (total_amount >= 0),
  hosted_invoice_url TEXT,
  period_started_at TIMESTAMPTZ,
  period_ends_at TIMESTAMPTZ,
  due_at TIMESTAMPTZ,
  paid_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(provider,provider_invoice_id)
);

CREATE INDEX billing_invoices_tenant_idx ON billing_invoices(tenant_id,created_at DESC);
CREATE INDEX billing_invoices_status_idx ON billing_invoices(status,due_at);

CREATE TABLE billing_webhook_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  provider TEXT NOT NULL,
  provider_event_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256)=64),
  status TEXT NOT NULL DEFAULT 'RECEIVED' CHECK (status IN ('RECEIVED','PROCESSED','FAILED')),
  error_message TEXT NOT NULL DEFAULT '',
  received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  processed_at TIMESTAMPTZ,
  UNIQUE(provider,provider_event_id)
);

CREATE INDEX billing_webhook_events_status_idx ON billing_webhook_events(status,received_at);

CREATE TABLE billing_subscription_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  webhook_event_id UUID REFERENCES billing_webhook_events(id) ON DELETE SET NULL,
  event_type TEXT NOT NULL,
  from_status TEXT,
  to_status TEXT NOT NULL,
  plan_key TEXT NOT NULL REFERENCES saas_plans(key) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX billing_subscription_events_tenant_idx ON billing_subscription_events(tenant_id,created_at DESC);
