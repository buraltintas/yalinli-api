#!/usr/bin/env bash
set -euo pipefail

API_BASE="${API_BASE:-http://localhost:${PORT:-8080}}"
BFF_SECRET="${BFF_SECRET:?BFF_SECRET is required}"

EMAIL="${SMOKE_EMAIL:-smoke+$(date +%s)@example.com}"
ACCESS_TOKEN="${ACCESS_TOKEN:-}"
PETITION_ID="${PETITION_ID:-}"
ENTRY_ID="${ENTRY_ID:-}"
MEDIA_ID="${MEDIA_ID:-}"

json() { jq -r "$1"; }

echo "[1] Health"
curl -sS -X GET "$API_BASE/v1/health" -H "X-BFF-SECRET: $BFF_SECRET" | jq .

echo "[2] Signup request code"
curl -sS -X POST "$API_BASE/v1/auth/signup/request-code" \
  -H "Content-Type: application/json" \
  -H "X-BFF-SECRET: $BFF_SECRET" \
  -d "{\"email\":\"$EMAIL\"}" | jq .

echo "[3] Verify code (manual step)"
echo "Run manually after reading OTP from email/logs:"
echo "curl -X POST $API_BASE/v1/auth/verify-code -H 'Content-Type: application/json' -H 'X-BFF-SECRET: $BFF_SECRET' -d '{\"email\":\"$EMAIL\",\"code\":\"123456\",\"purpose\":\"signup\"}'"

echo "[4] Create entry (requires ACCESS_TOKEN env)"
if [[ -n "$ACCESS_TOKEN" ]]; then
  curl -sS -X POST "$API_BASE/v1/entries" \
    -H "Content-Type: application/json" \
    -H "X-BFF-SECRET: $BFF_SECRET" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -d '{"type":"problem","category":"road","title":"Smoke entry","description":"Smoke test description","language":"tr","termsAccepted":true}' | tee /tmp/yalinli-entry.json | jq .
  ENTRY_ID="$(cat /tmp/yalinli-entry.json | jq -r '.id // empty')"
else
  echo "SKIP: ACCESS_TOKEN not set"
fi

echo "[5] Upload media"
if [[ -f "${SMOKE_IMAGE_PATH:-}" ]]; then
  curl -sS -X POST "$API_BASE/v1/media/upload" \
    -H "X-BFF-SECRET: $BFF_SECRET" \
    -F "file=@${SMOKE_IMAGE_PATH}" | tee /tmp/yalinli-media.json | jq .
  MEDIA_ID="$(cat /tmp/yalinli-media.json | jq -r '.id // empty')"
else
  echo "SKIP: set SMOKE_IMAGE_PATH=/abs/path/image.jpg"
fi

echo "[5b] Create entry with mediaIds (requires ACCESS_TOKEN + MEDIA_ID)"
if [[ -n "$ACCESS_TOKEN" && -n "$MEDIA_ID" ]]; then
  curl -sS -X POST "$API_BASE/v1/entries" \
    -H "Content-Type: application/json" \
    -H "X-BFF-SECRET: $BFF_SECRET" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -d "{\"type\":\"beauty\",\"category\":\"nature\",\"title\":\"Media Entry\",\"description\":\"Entry with media\",\"language\":\"tr\",\"termsAccepted\":true,\"mediaIds\":[\"$MEDIA_ID\"]}" | jq .
else
  echo "SKIP: ACCESS_TOKEN and MEDIA_ID required"
fi

echo "[6] Get feed"
curl -sS -X GET "$API_BASE/v1/feed" -H "X-BFF-SECRET: $BFF_SECRET" | jq .

echo "[7] Create petition (requires ACCESS_TOKEN)"
if [[ -n "$ACCESS_TOKEN" ]]; then
  curl -sS -X POST "$API_BASE/v1/petitions" \
    -H "Content-Type: application/json" \
    -H "X-BFF-SECRET: $BFF_SECRET" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -d '{"category":"municipality","title":"Smoke Petition","description":"Smoke petition description","language":"tr","termsAccepted":true}' | tee /tmp/yalinli-petition.json | jq .
  PETITION_ID="$(cat /tmp/yalinli-petition.json | jq -r '.id // empty')"
else
  echo "SKIP: ACCESS_TOKEN not set"
fi

echo "[8] Sign petition (requires ACCESS_TOKEN + PETITION_ID)"
if [[ -n "$ACCESS_TOKEN" && -n "$PETITION_ID" ]]; then
  curl -sS -X POST "$API_BASE/v1/petitions/$PETITION_ID/sign" \
    -H "Content-Type: application/json" \
    -H "X-BFF-SECRET: $BFF_SECRET" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -d '{"visibility":"public_name","termsAccepted":true}' | jq .
else
  echo "SKIP: ACCESS_TOKEN and PETITION_ID required"
fi

echo "[9] Export petition data (creator-only; requires ACCESS_TOKEN + PETITION_ID)"
if [[ -n "$ACCESS_TOKEN" && -n "$PETITION_ID" ]]; then
  curl -sS -X GET "$API_BASE/v1/petitions/$PETITION_ID/export-data" \
    -H "X-BFF-SECRET: $BFF_SECRET" \
    -H "Authorization: Bearer $ACCESS_TOKEN" | jq .
else
  echo "SKIP: ACCESS_TOKEN and PETITION_ID required"
fi

echo "Smoke-test flow complete."
