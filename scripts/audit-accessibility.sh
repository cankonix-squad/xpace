#!/bin/sh
set -eu

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
css="$root_dir/apps/web/src/app/globals.css"
dashboard="$root_dir/apps/web/src/app/page.tsx"
login="$root_dir/apps/web/src/app/login/page.tsx"
prejoin="$root_dir/apps/web/src/app/meet/[code]/prejoin/page.tsx"
network="$root_dir/apps/web/src/app/meet/[code]/room/network-status.tsx"

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

require_text "$css" 'select:focus-visible' "keyboard focus for select controls"
require_text "$css" 'a:focus-visible' "keyboard focus for links"
require_text "$css" '@media(prefers-reduced-motion:reduce)' "reduced-motion preference"
require_text "$dashboard" 'aria-label="Main navigation"' "named primary navigation"
require_text "$dashboard" 'aria-label="Open Xpace help"' "named help control"
require_text "$dashboard" 'role="dialog" aria-modal="true"' "modal semantics"
require_text "$dashboard" 'role="status"' "dashboard status announcement"
require_text "$login" 'role="alert"' "login error announcement"
require_text "$prejoin" 'aria-label={micOn?' "microphone control name"
require_text "$prejoin" 'aria-label={cameraOn?' "camera control name"
require_text "$network" '<strong>{label}</strong>' "network quality has a text label"
require_text "$network" 'Camera paused · audio prioritized · reduced data usage' "low-bandwidth status is not color-only"

echo "Accessibility source audit passed."
