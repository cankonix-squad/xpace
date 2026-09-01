ALTER TABLE email_outbox
  DROP CONSTRAINT IF EXISTS email_outbox_template_check;

ALTER TABLE email_outbox
  ALTER COLUMN token_encrypted DROP NOT NULL,
  ALTER COLUMN token_encrypted SET DEFAULT '',
  ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS dedupe_key TEXT;

UPDATE email_outbox SET token_encrypted='' WHERE token_encrypted IS NULL;

ALTER TABLE email_outbox
  ADD CONSTRAINT email_outbox_template_check
  CHECK (template IN ('VERIFY_EMAIL', 'RESET_PASSWORD', 'INVITATION', 'BILLING_NOTICE', 'SECURITY_NOTICE'));

CREATE UNIQUE INDEX IF NOT EXISTS email_outbox_dedupe_idx
  ON email_outbox (dedupe_key)
  WHERE dedupe_key IS NOT NULL;
