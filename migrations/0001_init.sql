-- +goose Up
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT UNIQUE NOT NULL,
  display_name TEXT,
  preferred_language TEXT NOT NULL DEFAULT 'tr',
  email_verified_at TIMESTAMPTZ,
  email_notifications_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  notify_new_entries BOOLEAN NOT NULL DEFAULT TRUE,
  notify_resolved_problems BOOLEAN NOT NULL DEFAULT TRUE,
  notify_new_petitions BOOLEAN NOT NULL DEFAULT TRUE,
  notify_new_events BOOLEAN NOT NULL DEFAULT TRUE,
  notify_announcements BOOLEAN NOT NULL DEFAULT TRUE,
  privacy_accepted_at TIMESTAMPTZ,
  privacy_version TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE auth_codes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT NOT NULL,
  purpose TEXT NOT NULL,
  code_hash TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  max_attempts INT NOT NULL,
  attempt_count INT NOT NULL DEFAULT 0,
  consumed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_auth_codes_email_created ON auth_codes(email, created_at DESC);

CREATE TABLE refresh_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  token_hash TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);

CREATE TABLE entries (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  type TEXT NOT NULL,
  category TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  language TEXT NOT NULL,
  status TEXT NOT NULL,
  user_id UUID REFERENCES users(id),
  display_name TEXT,
  is_anonymous BOOLEAN NOT NULL DEFAULT FALSE,
  anonymous_display_date TIMESTAMPTZ,
  contact_email TEXT,
  location_text TEXT,
  event_date TIMESTAMPTZ,
  resolved_at TIMESTAMPTZ,
  terms_accepted_at TIMESTAMPTZ NOT NULL,
  terms_version TEXT NOT NULL,
  privacy_accepted_at TIMESTAMPTZ,
  privacy_version TEXT,
  ip_hash TEXT,
  user_agent_hash TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_entries_feed ON entries(status, created_at DESC);

CREATE TABLE media (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  entry_id UUID REFERENCES entries(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'approved',
  mime_type TEXT NOT NULL,
  url TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE comments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  entry_id UUID NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
  user_id UUID REFERENCES users(id),
  body TEXT NOT NULL,
  language TEXT NOT NULL,
  display_name TEXT,
  is_anonymous BOOLEAN NOT NULL DEFAULT FALSE,
  anonymous_display_date TIMESTAMPTZ,
  status TEXT NOT NULL,
  terms_accepted_at TIMESTAMPTZ NOT NULL,
  terms_version TEXT NOT NULL,
  ip_hash TEXT,
  user_agent_hash TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_comments_entry_created ON comments(entry_id, created_at DESC);

CREATE TABLE petitions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  entry_id UUID UNIQUE NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
  status TEXT NOT NULL,
  created_by UUID NOT NULL REFERENCES users(id),
  signature_count INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE petition_signatures (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  petition_id UUID NOT NULL REFERENCES petitions(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  visibility TEXT NOT NULL,
  terms_accepted_at TIMESTAMPTZ NOT NULL,
  terms_version TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(petition_id, user_id)
);

CREATE TABLE reports (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  target_type TEXT NOT NULL,
  target_id UUID NOT NULL,
  reason TEXT NOT NULL,
  description TEXT,
  reporter_email TEXT,
  ip_hash TEXT,
  user_agent_hash TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE email_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES users(id),
  email_to TEXT NOT NULL,
  template_key TEXT NOT NULL,
  language TEXT NOT NULL,
  status TEXT NOT NULL,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS email_logs;
DROP TABLE IF EXISTS reports;
DROP TABLE IF EXISTS petition_signatures;
DROP TABLE IF EXISTS petitions;
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS media;
DROP TABLE IF EXISTS entries;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS auth_codes;
DROP TABLE IF EXISTS users;
