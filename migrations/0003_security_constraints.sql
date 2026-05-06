-- +goose Up
ALTER TABLE users
  ADD CONSTRAINT users_preferred_language_check CHECK (preferred_language IN ('tr', 'en')) NOT VALID;

ALTER TABLE auth_codes
  ADD CONSTRAINT auth_codes_purpose_check CHECK (purpose IN ('signup', 'login')) NOT VALID,
  ADD CONSTRAINT auth_codes_attempts_check CHECK (max_attempts > 0 AND attempt_count >= 0 AND attempt_count <= max_attempts) NOT VALID;

ALTER TABLE entries
  ADD CONSTRAINT entries_type_check CHECK (type IN ('problem', 'beauty', 'request', 'suggestion', 'event', 'resolved_problem', 'announcement', 'petition')) NOT VALID,
  ADD CONSTRAINT entries_status_check CHECK (status IN ('approved', 'pending_review', 'reported', 'hidden', 'deleted')) NOT VALID,
  ADD CONSTRAINT entries_language_check CHECK (language IN ('tr', 'en')) NOT VALID,
  ADD CONSTRAINT entries_title_length_check CHECK (char_length(btrim(title)) BETWEEN 1 AND 160) NOT VALID,
  ADD CONSTRAINT entries_description_length_check CHECK (char_length(btrim(description)) BETWEEN 1 AND 4000) NOT VALID,
  ADD CONSTRAINT entries_category_length_check CHECK (char_length(btrim(category)) BETWEEN 1 AND 80) NOT VALID;

ALTER TABLE media
  ADD CONSTRAINT media_status_check CHECK (status IN ('approved', 'pending_review', 'reported', 'hidden', 'deleted')) NOT VALID,
  ADD CONSTRAINT media_mime_type_check CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp')) NOT VALID;

ALTER TABLE comments
  ADD CONSTRAINT comments_status_check CHECK (status IN ('approved', 'pending_review', 'reported', 'hidden', 'deleted')) NOT VALID,
  ADD CONSTRAINT comments_language_check CHECK (language IN ('tr', 'en')) NOT VALID,
  ADD CONSTRAINT comments_body_length_check CHECK (char_length(btrim(body)) BETWEEN 1 AND 2000) NOT VALID;

ALTER TABLE petitions
  ADD CONSTRAINT petitions_status_check CHECK (status IN ('approved', 'pending_review', 'closed', 'hidden', 'deleted')) NOT VALID;

ALTER TABLE petition_signatures
  ADD CONSTRAINT petition_signatures_visibility_check CHECK (visibility IN ('public_name', 'anonymous')) NOT VALID,
  ADD CONSTRAINT petition_signatures_snapshot_check CHECK (
    signer_full_name_snapshot IS NOT NULL
    AND char_length(btrim(signer_full_name_snapshot)) BETWEEN 1 AND 160
    AND signer_email_snapshot IS NOT NULL
    AND char_length(btrim(signer_email_snapshot)) > 0
    AND masked_signer_name IS NOT NULL
    AND char_length(btrim(masked_signer_name)) > 0
    AND signature_hash IS NOT NULL
    AND char_length(signature_hash) = 64
  ) NOT VALID;

ALTER TABLE reports
  ADD CONSTRAINT reports_target_type_check CHECK (target_type IN ('entry', 'comment', 'media')) NOT VALID,
  ADD CONSTRAINT reports_reason_check CHECK (reason IN ('personal_data', 'insult_or_defamation', 'privacy', 'false_information', 'spam', 'violence_or_threat', 'copyright', 'other')) NOT VALID;

-- +goose Down
ALTER TABLE reports
  DROP CONSTRAINT IF EXISTS reports_reason_check,
  DROP CONSTRAINT IF EXISTS reports_target_type_check;

ALTER TABLE petition_signatures
  DROP CONSTRAINT IF EXISTS petition_signatures_snapshot_check,
  DROP CONSTRAINT IF EXISTS petition_signatures_visibility_check;

ALTER TABLE petitions
  DROP CONSTRAINT IF EXISTS petitions_status_check;

ALTER TABLE comments
  DROP CONSTRAINT IF EXISTS comments_body_length_check,
  DROP CONSTRAINT IF EXISTS comments_language_check,
  DROP CONSTRAINT IF EXISTS comments_status_check;

ALTER TABLE media
  DROP CONSTRAINT IF EXISTS media_mime_type_check,
  DROP CONSTRAINT IF EXISTS media_status_check;

ALTER TABLE entries
  DROP CONSTRAINT IF EXISTS entries_category_length_check,
  DROP CONSTRAINT IF EXISTS entries_description_length_check,
  DROP CONSTRAINT IF EXISTS entries_title_length_check,
  DROP CONSTRAINT IF EXISTS entries_language_check,
  DROP CONSTRAINT IF EXISTS entries_status_check,
  DROP CONSTRAINT IF EXISTS entries_type_check;

ALTER TABLE auth_codes
  DROP CONSTRAINT IF EXISTS auth_codes_attempts_check,
  DROP CONSTRAINT IF EXISTS auth_codes_purpose_check;

ALTER TABLE users
  DROP CONSTRAINT IF EXISTS users_preferred_language_check;
