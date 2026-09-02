#!/bin/sh
set -eu

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
css="$root_dir/apps/web/src/app/globals.css"
dashboard="$root_dir/apps/web/src/app/page.tsx"
shell="$root_dir/apps/web/src/components/workspace-shell.tsx"
search="$root_dir/apps/web/src/components/global-search.tsx"
notifications="$root_dir/apps/web/src/components/notification-center.tsx"
account="$root_dir/apps/web/src/components/account-menu.tsx"
login="$root_dir/apps/web/src/app/login/page.tsx"
prejoin="$root_dir/apps/web/src/app/meet/[code]/prejoin/page.tsx"
network="$root_dir/apps/web/src/app/meet/[code]/room/network-status.tsx"
room="$root_dir/apps/web/src/app/meet/[code]/room/page.tsx"
users="$root_dir/apps/web/src/app/admin/users/page.tsx"
invitation="$root_dir/apps/web/src/app/accept-invite/page.tsx"

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
require_text "$dashboard" 'aria-label="Open Xspace help"' "named help control"
require_text "$dashboard" 'role="dialog"' "modal dialog role"
require_text "$dashboard" 'aria-modal="true"' "modal isolation semantics"
require_text "$dashboard" 'role="status"' "dashboard status announcement"
require_text "$shell" 'aria-label="Main navigation"' "shared workspace navigation name"
require_text "$shell" 'aria-label="Open navigation"' "shared mobile navigation control"
require_text "$search" 'role="combobox"' "global search combobox semantics"
require_text "$search" 'aria-controls="global-search-results"' "global search result relationship"
require_text "$notifications" 'aria-expanded={open}' "notification expansion state"
require_text "$notifications" 'role="alert"' "notification error announcement"
require_text "$account" 'aria-expanded={open}' "account menu expansion state"
require_text "$account" 'aria-label="Account navigation"' "account navigation name"
require_text "$login" 'role="alert"' "login error announcement"
require_text "$prejoin" '"Turn microphone off"' "microphone-off control name"
require_text "$prejoin" '"Turn microphone on"' "microphone-on control name"
require_text "$prejoin" '"Turn camera off"' "camera-off control name"
require_text "$prejoin" '"Turn camera on"' "camera-on control name"
require_text "$network" '<strong>{label}</strong>' "network quality has a text label"
require_text "$network" 'Camera paused · audio prioritized · reduced data usage' "low-bandwidth status is not color-only"
require_text "$room" 'aria-label="Open meeting actions"' "named mobile meeting actions"
require_text "$room" 'aria-expanded={moreOpen}' "meeting action expansion state"
require_text "$room" 'aria-label="Network diagnostics"' "named network diagnostics panel"
require_text "$room" 'aria-expanded={diagnosticsOpen}' "diagnostics expansion state"
require_text "$users" 'aria-label="Account creation mode"' "named account creation mode"
require_text "$users" 'aria-pressed={mode === "ACTIVE"}' "active user mode pressed state"
require_text "$users" 'role={error ? "alert" : "status"}' "user management live messages"
require_text "$users" 'autoComplete="new-password"' "new-user password autocomplete"
require_text "$invitation" 'role="alert"' "invitation error announcement"
require_text "$invitation" 'autoComplete="new-password"' "invitation password autocomplete"
require_text "$css" 'font-family:Helvetica,Arial,sans-serif' "Helvetica platform typography"

if rg -n --pcre2 'font-size:(?:[0-9](?:\.[0-9]+)?|1[01](?:\.[0-9]+)?)px|fontSize:(?:[0-9](?:\.[0-9]+)?|1[01](?:\.[0-9]+)?)(?![0-9])' "$root_dir/apps/web/src" -g '*.css' -g '*.tsx'; then
  echo "FAIL: typography smaller than 12px remains"
  exit 1
fi
echo "PASS: platform typography minimum is 12px"

if rg -n 'font-family:(?!Helvetica,Arial,sans-serif)' "$root_dir/apps/web/src" -g '*.css' --pcre2; then
  echo "FAIL: non-Helvetica font family remains"
  exit 1
fi
echo "PASS: all explicit font families use Helvetica"

echo "Accessibility source audit passed."
