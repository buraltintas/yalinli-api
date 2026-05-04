-- +goose Up
ALTER TABLE users ADD COLUMN IF NOT EXISTS full_name TEXT;

ALTER TABLE petitions
  ADD COLUMN IF NOT EXISTS target_authority TEXT,
  ADD COLUMN IF NOT EXISTS requested_action TEXT,
  ADD COLUMN IF NOT EXISTS location_text TEXT,
  ADD COLUMN IF NOT EXISTS deadline_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS closed_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE petition_signatures
  ADD COLUMN IF NOT EXISTS signer_full_name_snapshot TEXT,
  ADD COLUMN IF NOT EXISTS signer_email_snapshot TEXT,
  ADD COLUMN IF NOT EXISTS masked_signer_name TEXT,
  ADD COLUMN IF NOT EXISTS signed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS signature_hash TEXT,
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE TABLE IF NOT EXISTS petition_export_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  petition_id UUID NOT NULL REFERENCES petitions(id) ON DELETE CASCADE,
  exported_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  signature_count_at_export INT NOT NULL,
  exported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ip_hash TEXT,
  user_agent_hash TEXT
);

-- +goose Down
DROP TABLE IF EXISTS petition_export_logs;
ALTER TABLE petition_signatures
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS signature_hash,
  DROP COLUMN IF EXISTS signed_at,
  DROP COLUMN IF EXISTS masked_signer_name,
  DROP COLUMN IF EXISTS signer_email_snapshot,
  DROP COLUMN IF EXISTS signer_full_name_snapshot;
ALTER TABLE petitions
  DROP COLUMN IF EXISTS published_at,
  DROP COLUMN IF EXISTS closed_at,
  DROP COLUMN IF EXISTS deadline_at,
  DROP COLUMN IF EXISTS location_text,
  DROP COLUMN IF EXISTS requested_action,
  DROP COLUMN IF EXISTS target_authority;
ALTER TABLE users DROP COLUMN IF EXISTS full_name;
