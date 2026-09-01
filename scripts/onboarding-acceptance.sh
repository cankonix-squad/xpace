#!/bin/sh
set -eu

api_url="${API_URL:-http://127.0.0.1:8080}"
slug="acceptance-$(date +%s)"
email="${slug}@example.com"
old_password='Strong!Pass-2026'
new_password='Changed!Pass-2026'
cookie_file="$(mktemp)"
trap 'rm -f "$cookie_file"' EXIT

signup_response="$(curl -fsS -X POST "$api_url/api/v1/auth/signup" -H 'Content-Type: application/json' --data "{\"tenantName\":\"Acceptance Workspace\",\"tenantSlug\":\"$slug\",\"displayName\":\"Acceptance Owner\",\"email\":\"$email\",\"username\":\"owner\",\"password\":\"$old_password\",\"passwordConfirm\":\"$old_password\",\"termsAccepted\":true}")"
verification_token="$(printf '%s' "$signup_response" | jq -er '.developmentToken')"

status="$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$api_url/api/v1/auth/login" -H 'Content-Type: application/json' --data "{\"tenant\":\"$slug\",\"identity\":\"owner\",\"password\":\"$old_password\"}")"
[ "$status" = 401 ] || { echo "expected pre-verification login 401, got $status" >&2; exit 1; }

curl -fsS -o /dev/null -X POST "$api_url/api/v1/auth/verify-email" -H 'Content-Type: application/json' --data "{\"token\":\"$verification_token\"}"
curl -fsS -c "$cookie_file" -o /dev/null -X POST "$api_url/api/v1/auth/login" -H 'Content-Type: application/json' --data "{\"tenant\":\"$slug\",\"identity\":\"owner\",\"password\":\"$old_password\"}"
curl -fsS -b "$cookie_file" -o /dev/null "$api_url/api/v1/auth/me"

forgot_response="$(curl -fsS -X POST "$api_url/api/v1/auth/forgot-password" -H 'Content-Type: application/json' --data "{\"tenant\":\"$slug\",\"email\":\"$email\"}")"
reset_token="$(printf '%s' "$forgot_response" | jq -er '.developmentToken')"
curl -fsS -o /dev/null -X POST "$api_url/api/v1/auth/reset-password" -H 'Content-Type: application/json' --data "{\"token\":\"$reset_token\",\"password\":\"$new_password\",\"passwordConfirm\":\"$new_password\"}"

status="$(curl -sS -o /dev/null -w '%{http_code}' -b "$cookie_file" "$api_url/api/v1/auth/me")"
[ "$status" = 401 ] || { echo "expected old session 401, got $status" >&2; exit 1; }
status="$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$api_url/api/v1/auth/login" -H 'Content-Type: application/json' --data "{\"tenant\":\"$slug\",\"identity\":\"owner\",\"password\":\"$old_password\"}")"
[ "$status" = 401 ] || { echo "expected old password 401, got $status" >&2; exit 1; }
curl -fsS -o /dev/null -X POST "$api_url/api/v1/auth/login" -H 'Content-Type: application/json' --data "{\"tenant\":\"$slug\",\"identity\":\"owner\",\"password\":\"$new_password\"}"

printf 'onboarding acceptance passed tenant=%s email=%s\n' "$slug" "$email"
