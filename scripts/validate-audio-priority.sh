#!/bin/sh
set -eu

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
room_page="$root_dir/apps/web/src/app/meet/[code]/room/page.tsx"
network_status="$root_dir/apps/web/src/app/meet/[code]/room/network-status.tsx"
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

require_text "$room_page" 'dtx:true' "Opus discontinuous transmission enabled"
require_text "$room_page" 'red:true' "redundant audio packets enabled"
require_text "$track_options" 'dtx?: boolean;' "installed SDK supports DTX"
require_text "$track_options" 'red?: boolean;' "installed SDK supports RED"
require_text "$network_status" 'localParticipant.setCameraEnabled(false)' "low-bandwidth mode pauses camera"
require_text "$network_status" 'setProfile("Audio priority")' "audio-priority state is exposed"
require_text "$network_status" 'resolution:{width:640,height:360},frameRate:15' "camera recovers at 360p and 15 FPS"
require_text "$network_status" 'Camera paused · audio prioritized · reduced data usage' "user receives non-color status feedback"

if grep -Fq 'setMicrophoneEnabled(false)' "$network_status"; then
  echo "FAIL: audio-priority mode disables the microphone"
  exit 1
fi
echo "PASS: microphone remains published in audio-priority mode"

echo "Audio-priority validation passed."
