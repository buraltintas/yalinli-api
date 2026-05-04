# yalinli-api

Go backend API for the Döşemealtı / Antalya / Yalınlı neighborhood platform.

## Quick Start

1. Copy `.env.example` to `.env` and fill required values.
2. Start PostgreSQL.
3. Run migrations: `make migrate-up`
4. Run API: `make run`

Server listens on `PORT` (default `8080`).

## Reused Architecture Note

See [docs/architecture.md](/Users/burakaltintas/Documents/yalinli-api/docs/architecture.md).

## Required Env Vars

- `APP_ENV`
- `PORT`
- `DATABASE_URL`
- `BFF_SECRET`
- `JWT_SECRET`
- `JWT_ACCESS_TOKEN_TTL_MINUTES`
- `JWT_REFRESH_TOKEN_TTL_DAYS`
- `CORS_ALLOWED_ORIGINS`
- `DEFAULT_LANGUAGE=tr`
- `SUPPORTED_LANGUAGES=tr,en`
- `CONTENT_REVIEW_MODE=auto_approve`
- `CURRENT_TERMS_VERSION=2026-05-04-v1`
- `CURRENT_PRIVACY_VERSION=2026-05-04-v1`
- `EMAIL_FROM`
- `EMAIL_FROM_NAME`
- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USERNAME`
- `SMTP_PASSWORD`
- `SMTP_USE_TLS`
- `OTP_CODE_TTL_MINUTES=10`
- `OTP_MAX_ATTEMPTS=5`
- `MEDIA_MAX_IMAGES_PER_ENTRY=3`
- `MEDIA_MAX_FILE_SIZE_MB=5`
- `STORAGE_BACKEND=gcs`
- `GCP_PROJECT_ID=yalinli`
- `GCS_BUCKET=yalinli-images-a1b2c3`
- `GCS_UPLOAD_PREFIX=yalinli`
- `GCS_SIGNED_URL_TTL=24h`
- `SIGNATURE_HASH_SECRET`

## BFF Secret

All `/v1/*` endpoints require header:

`X-BFF-SECRET: <BFF_SECRET>`

## Main Endpoints

- `GET /v1/health`
- Auth:
  - `POST /v1/auth/signup/request-code`
  - `POST /v1/auth/login/request-code`
  - `POST /v1/auth/verify-code`
  - `POST /v1/auth/refresh`
  - `POST /v1/auth/logout`
- User:
  - `GET /v1/me`
  - `PATCH /v1/me`
  - `GET /v1/me/notification-preferences`
  - `PATCH /v1/me/notification-preferences`
  - `POST /v1/unsubscribe`
- Feed/entries:
  - `GET /v1/feed`
  - `GET /v1/entries/:id`
  - `POST /v1/entries`
  - `POST /v1/entries` accepts optional `mediaIds: string[]` (max 3)
- Comments:
  - `GET /v1/entries/:id/comments`
  - `POST /v1/entries/:id/comments`
- Media:
  - `POST /v1/media/upload`
  - Returns `id` and signed `url` (internal storage ref is never returned)
- Petitions:
  - `GET /v1/petitions`
  - `GET /v1/petitions/:id`
  - `POST /v1/petitions`
  - `POST /v1/petitions/:id/sign`
  - `GET /v1/petitions/:id/export-data`
  - `DELETE /v1/petitions/:id/sign`
- Reports:
  - `POST /v1/reports`

## Language Resolution

Priority:
1. `?lang=tr|en`
2. Authenticated user preferred language (planned improvement in middleware)
3. `Accept-Language`
4. default `tr`

## Content Review Mode

- `auto_approve` -> new content status: `approved`
- `manual_review` -> new content status: `pending_review`

## Storage (GCS Default)

- Default storage backend is Google Cloud Storage.
- Uploaded media is stored in GCS and returned as signed URLs.
- `MEDIA_PUBLIC_BASE_URL` and `STORAGE_LOCAL_DIR` are only for optional local fallback development mode (`STORAGE_BACKEND=local`).
- Cloud Run setup should use:
  - `STORAGE_BACKEND=gcs`
  - `GCP_PROJECT_ID=yalinli`
  - `GCS_BUCKET=yalinli-images-a1b2c3`
  - `GCS_UPLOAD_PREFIX=yalinli`
  - `GCS_SIGNED_URL_TTL=24h`

## Updated Implementation Plan (Petitions & Signatures)

1. Authorization and visibility rules
   - Keep petition listing/detail public (`GET /v1/petitions`, `GET /v1/petitions/:id`).
   - Require authenticated user for petition creation (`POST /v1/petitions`).
   - Require authenticated user for signing (`POST /v1/petitions/:id/sign`).
   - Keep `X-BFF-SECRET` required on all `/v1/*` endpoints.
   - Enforce creator-only export access on `GET /v1/petitions/:id/export-data`; non-creators return `FORBIDDEN`.
2. Petition model alignment
   - Use petition fields:
     - `id`, `title`, `description`, `language`, `status`, `createdByUserId`, `targetAuthority`, `requestedAction`, `locationText`, `deadlineAt`, `closedAt`, `termsAcceptedAt`, `termsVersion`, `createdAt`, `updatedAt`, `publishedAt`.
   - Status set must support `approved`, `pending_review`, `closed`, `hidden`, `deleted`.
   - Creation status follows `CONTENT_REVIEW_MODE`.
3. User model update for signing
   - Add optional `fullName` to users.
   - Keep `displayName` for public display.
   - Petition signing requires full name:
     - If profile `fullName` is missing, require `fullName` in sign request.
     - Optional `updateProfileFullName=true` updates profile `fullName`.
4. Petition signature model and constraints
   - Use `petition_signatures` fields:
     - `id`, `petitionId`, `userId`, `signerFullNameSnapshot`, `signerEmailSnapshot`, `maskedSignerName`, `signedAt`, `electronicApprovalTextVersion`, `signatureTermsAcceptedAt`, `signatureTermsVersion`, `ipHash`, `userAgentHash`, `signatureHash`, `createdAt`, `updatedAt`.
   - Add unique constraint on `(petitionId, userId)`.
   - Require `signerFullNameSnapshot`.
   - Snapshot full name and email at signing time; never rely on mutable profile values for export.
   - Store `maskedSignerName` at signing time.
   - Store `signerEmailSnapshot` for audit/export only; never public.
5. Signature hash and secrets
   - Add env var `SIGNATURE_HASH_SECRET`.
   - Generate `signatureHash` via server-side HMAC from:
     - `petitionId`, `userId`, `signedAt`, `signerEmailSnapshot`, `signatureTermsVersion`, `SIGNATURE_HASH_SECRET`.
   - Never expose raw hash input or secret.
6. Signing endpoint behavior (`POST /v1/petitions/:id/sign`)
   - Require authenticated user and valid `X-BFF-SECRET`.
   - Accept:
     - `fullName` (required if profile full name missing)
     - `signatureTermsAccepted=true` (required)
     - `updateProfileFullName=true|false` (optional)
   - Petition must be `approved`; reject `closed`, `hidden`, `deleted`.
   - Reject duplicates with `PETITION_ALREADY_SIGNED`.
   - Store snapshots, masked name, signature hash, and localized response message.
7. Public petition detail behavior (`GET /v1/petitions/:id`)
   - Return public petition fields and `signatureCount`.
   - Return masked signer list (limited/paginated).
   - Optionally return current authenticated user signing status if auth exists.
   - Never return signer full names, signer emails, IP/user-agent hashes, or signature hashes.
8. Masking rules
   - Example masked names:
     - `B**** A*******`
     - `M***** Y*****`
   - If safe masking is not possible, fallback:
     - TR: `Maskeli Kullanıcı`
     - EN: `Masked User`
9. Export-data endpoint (`GET /v1/petitions/:id/export-data`)
   - Require authenticated user and valid `X-BFF-SECRET`.
   - Allow only petition creator.
   - Add strict rate limiting.
   - Log each export in `petition_export_logs`:
     - `id`, `petitionId`, `exportedByUserId`, `signatureCountAtExport`, `exportedAt`, `ipHash`, `userAgentHash`.
   - Return:
     - Petition metadata, creator info, `createdAt`, `exportedAt`, `signatureCount`.
     - Signature rows with: `rowNumber`, `signerFullName`, `signerEmail`, `signedAt`, electronic approval text, wet signature label, empty wet signature placeholder, and safe verification value (`signatureHash` or short verification code).
     - Localized legal note.
10. Required legal wording (`yalinli.org`)
   - Do not classify approval as qualified electronic signature.
   - Track that support was electronically approved via yalinli.org.
   - Electronic approval text:
     - TR: `yalinli.org üzerinden elektronik olarak onaylanmıştır.`
     - EN: `Electronically approved through yalinli.org.`
   - Wet signature labels:
     - TR: `Islak İmza`
     - EN: `Wet Signature`
   - Export legal note:
     - TR: `Bu kayıt, nitelikli elektronik imza veya ıslak imza yerine geçmez; kampanyaya yalinli.org üzerinden verilen elektronik onayı gösterir.`
     - EN: `This record does not replace a qualified electronic signature or wet signature; it indicates electronic approval given through yalinli.org.`
11. Petition listing behavior (`GET /v1/petitions`)
   - Public endpoint.
   - Return only visible petitions (`approved` and optionally `closed`).
   - Exclude `hidden` and `deleted`.
   - Include `signatureCount`.
   - Include viewer signing status if auth exists.
12. Unsign behavior (`DELETE /v1/petitions/:id/sign`)
   - Optional for MVP.
   - If implemented, allow only when petition is not `closed`.
   - Prefer audit-safe soft-delete approach; otherwise skip.
13. Notification behavior
   - Send new petition notification only when petition becomes `approved`.
   - Respect both global `emailNotificationsEnabled=false` and petition preference toggle.
   - Localize by preferred language.
14. Test plan additions
   - Unauthenticated user cannot create petition.
   - Unauthenticated user can view petition.
   - Unauthenticated user cannot sign.
   - Authenticated user can sign once.
   - Duplicate signing is rejected.
   - Public petition response contains masked signer names only.
   - Public petition response never includes signer email/full name.
   - Only petition creator can access export-data.
   - Non-creator gets `FORBIDDEN`.
   - Export-data includes full name/email rows, yalinli.org approval text, legal note, and wet signature labels.
   - Export action creates `petition_export_logs` record.

## PDF Note

- Direct backend PDF generation is optional and can be skipped in MVP.
- Frontend can generate PDF from `GET /v1/petitions/:id/export-data`.

## Example cURL

```bash
curl -X GET http://localhost:8080/v1/health \
  -H "X-BFF-SECRET: $BFF_SECRET"
```

```bash
curl -X POST http://localhost:8080/v1/auth/signup/request-code \
  -H "Content-Type: application/json" \
  -H "X-BFF-SECRET: $BFF_SECRET" \
  -d '{"email":"test@example.com"}'
```

## Smoke Test Prep

- Script: `scripts/smoke-test.sh`
- Requires: `jq`, running API, and valid `BFF_SECRET`.
- Usage example:
  - `BFF_SECRET=... API_BASE=http://localhost:8080 ./scripts/smoke-test.sh`
- Optional vars:
  - `ACCESS_TOKEN` (for auth-required steps)
  - `SMOKE_IMAGE_PATH` (local test image path)
  - `MEDIA_ID` (if reusing uploaded media)
  - `PETITION_ID` (if reusing an existing petition)

## Email Setup

If `SMTP_HOST` is empty, email sending is safely skipped and only logged.

## Send Test Email (Ops/Dev)

Run:

```bash
go run ./cmd/send-test-email --to burak.altintas@yahoo.com.tr
```

Behavior:
- Loads `.env` and config exactly like the API.
- Uses `EMAIL_FROM`, `EMAIL_FROM_NAME`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_USE_TLS`.
- Sends subject: `yalinli.org test email`
- Sends body:
  - `Bu yalinli.org API tarafından gönderilen test e-postasıdır.`
  - `This is a test email sent by the yalinli.org API.`
- Fails clearly if `SMTP_HOST` is empty.

## Cloud Run Friendly

- `Dockerfile` is included.
- `cloudbuild.yaml` is included.

## Notes

- No admin panel in MVP.
- No manual moderation in MVP (but status fields and report flow are ready).
- User-generated texts are stored as-is and are not translated.
