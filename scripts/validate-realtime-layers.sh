#!/bin/sh
set -eu

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
room_page="$root_dir/apps/web/src/app/meet/[code]/room/page.tsx"
sdk_options="$root_dir/apps/web/node_modules/livekit-client/dist/src/options.d.ts"
track_options="$root_dir/apps/web/node_modules/livekit-client/dist/src/room/track/options.d.ts"

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

require_text "$room_page" 'adaptiveStream:{pixelDensity:"screen"}' "adaptive subscribed layers enabled"
require_text "$room_page" 'dynacast:true' "unused publish layers can be paused"
require_text "$room_page" 'simulcast:true' "camera simulcast enabled"
require_text "$room_page" 'scalabilityMode:"L3T3_KEY"' "three-spatial/three-temporal SVC configured"
require_text "$sdk_options" 'adaptiveStream: AdaptiveStreamSettings | boolean;' "installed SDK supports adaptive stream"
require_text "$sdk_options" 'dynacast: boolean;' "installed SDK supports dynacast"
require_text "$track_options" 'simulcast?: boolean;' "installed SDK supports simulcast"
require_text "$track_options" 'scalabilityMode?: ScalabilityMode;' "installed SDK supports SVC modes"

cd "$root_dir"
livekit_status=$(docker compose ps --format json livekit)
if ! printf '%s' "$livekit_status" | grep -q '"Health":"healthy"'; then
  echo "FAIL: LiveKit container is not healthy"
  exit 1
fi
echo "PASS: LiveKit container is healthy"

echo "Realtime layer validation passed."
