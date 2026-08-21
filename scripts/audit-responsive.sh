#!/bin/sh
set -eu

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
base_url=${XPACE_WEB_URL:-http://127.0.0.1:3300}
global_css="$root_dir/apps/web/src/app/globals.css"
admin_css="$root_dir/apps/web/src/app/admin/admin.module.css"
users_css="$root_dir/apps/web/src/app/admin/users/users.module.css"

require_text() {
  file=$1
  expected=$2
  label=$3
  if ! grep -Fq -- "$expected" "$file"; then
    echo "FAIL: $label"
    exit 1
  fi
  echo "PASS: $label"
}

require_text "$global_css" '@media(max-width:760px)' "dashboard mobile navigation breakpoint"
require_text "$global_css" '@media(max-width:430px)' "compact phone dashboard breakpoint"
require_text "$global_css" '@media(max-width:800px)' "single-column mobile login"
require_text "$global_css" '@media(max-width:850px)' "single-column tablet prejoin"
require_text "$global_css" '@media(max-width:480px)' "compact phone prejoin"
require_text "$global_css" '@media(max-width:600px){.host-panel' "mobile participant panel"
require_text "$admin_css" '@media(max-width:900px)' "scroll-safe admin tables"
require_text "$admin_css" '@media(max-width:520px)' "single-column phone admin"
require_text "$users_css" '@media(max-width:850px)' "single-column user administration"
require_text "$global_css" '@media(prefers-reduced-motion:reduce)' "reduced-motion compatibility"

for route in / /login /meet/KX0-YWQ-1R3/prejoin /admin; do
  status=$(curl -sS -o /dev/null -w '%{http_code}' "$base_url$route")
  if [ "$status" != "200" ]; then
    echo "FAIL: $route returned HTTP $status, expected 200"
    exit 1
  fi
  echo "PASS: $route returns HTTP 200"
done

permissions=$(curl -fsSI "$base_url/meet/KX0-YWQ-1R3/prejoin")
if ! printf '%s' "$permissions" | grep -qi 'Permissions-Policy: camera=(self), microphone=(self)'; then
  echo "FAIL: browser media permission policy"
  exit 1
fi
echo "PASS: browser camera and microphone permission policy"

echo "Responsive source and route audit passed."
