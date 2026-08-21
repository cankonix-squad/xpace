#!/bin/sh
set -eu

web_url=${XPACE_WEB_URL:-http://127.0.0.1:3300}
api_url=${XPACE_API_URL:-http://127.0.0.1:8080}
livekit_url=${XPACE_LIVEKIT_URL:-http://127.0.0.1:7880}
recording_url=${XPACE_RECORDING_URL:-http://127.0.0.1:9000/xpace-recordings/}
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

expect_status() {
  expected=$1
  url=$2
  label=$3
  actual=$(curl --connect-timeout 5 --max-time 15 -sS -o /dev/null -w '%{http_code}' "$url")
  if [ "$actual" != "$expected" ]; then
    echo "FAIL: $label returned HTTP $actual, expected $expected"
    exit 1
  fi
  echo "PASS: $label returned HTTP $expected"
}

expect_status 200 "$web_url/" "web dashboard"
expect_status 200 "$web_url/login" "login page"
expect_status 200 "$web_url/meet/KX0-YWQ-1R3/prejoin" "prejoin route"
expect_status 200 "$api_url/healthz" "API liveness"
expect_status 200 "$api_url/api/v1/health" "API readiness"
expect_status 200 "$livekit_url/" "LiveKit signaling"
if [ "$recording_url" = "skip" ]; then
  echo "SKIP: recording bucket has no public edge endpoint"
else
  expect_status 403 "$recording_url" "private recording bucket"
fi

curl --connect-timeout 5 --max-time 15 -fsS "$web_url/login" -o "$tmp_dir/login.html"
grep -Fq '<title>Xpace - Secure Collaboration' "$tmp_dir/login.html" || {
  echo "FAIL: expected Xpace document title was not rendered"
  exit 1
}
echo "PASS: Xpace document title rendered"

curl --connect-timeout 5 --max-time 15 -fsSI "$web_url/login" -o "$tmp_dir/web.headers"
for header in content-security-policy x-content-type-options referrer-policy permissions-policy; do
  if ! grep -qi "^$header:" "$tmp_dir/web.headers"; then
    echo "FAIL: missing web security header: $header"
    exit 1
  fi
done
echo "PASS: required web security headers are present"

curl --connect-timeout 5 --max-time 15 -fsS "$api_url/api/v1/health" -o "$tmp_dir/api-health.json"
grep -Fq '"status":"ready"' "$tmp_dir/api-health.json" || {
  echo "FAIL: API readiness payload is not ready"
  exit 1
}
echo "PASS: API reports ready"

echo "Xpace PoC smoke test passed."
