# Reused Architecture Decisions from `ux-anomalies-api`

- Keep Go API layout with `cmd/api`, `cmd/migrate`, `internal/*`, `migrations/*`.
- Keep middleware-first HTTP stack (request id, logging, recover, BFF guard, auth guard, rate limiting).
- Keep env-driven config loading with `.env` support.
- Keep PostgreSQL via `pgxpool` and Goose migrations.
- Keep JWT utility pattern for access/refresh tokens.
- Keep secure response envelope style (no stack traces in responses).
- Keep local/cloud-run friendly Docker + cloudbuild files.
- Adapt domain logic only (entries/comments/petitions/reports/otp), without copying unrelated UX anomaly entities.
