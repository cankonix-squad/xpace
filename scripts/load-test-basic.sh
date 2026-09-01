#!/bin/sh
set -eu

url=${XPACE_LOAD_URL:-https://xspace.cankonix.com/healthz}
requests=${XPACE_LOAD_REQUESTS:-200}
concurrency=${XPACE_LOAD_CONCURRENCY:-10}
max_failure_percent=${XPACE_LOAD_MAX_FAILURE_PERCENT:-1}
max_p95_ms=${XPACE_LOAD_MAX_P95_MS:-1000}

case "$requests:$concurrency:$max_failure_percent:$max_p95_ms" in
  *[!0-9:]*|:*) echo "load-test settings must be non-negative integers" >&2; exit 2 ;;
esac
[ "$requests" -gt 0 ] && [ "$concurrency" -gt 0 ] || { echo "requests and concurrency must be greater than zero" >&2; exit 2; }

results=$(mktemp)
trap 'rm -f "$results"' EXIT INT TERM

seq "$requests" | xargs -P "$concurrency" -n 1 sh -c '
  result=$(curl --connect-timeout 3 --max-time 10 -sS -o /dev/null -w "%{http_code} %{time_total}" "$1" 2>/dev/null) || result="000 10.000"
  printf "%s\n" "$result"
' _ "$url" > "$results"

completed=$(wc -l < "$results" | tr -d ' ')
failed=$(awk '$1 !~ /^2[0-9][0-9]$/ {count++} END {print count+0}' "$results")
failure_percent=$(awk -v failed="$failed" -v total="$completed" 'BEGIN {printf "%.2f", total ? failed*100/total : 100}')
p95_position=$(awk -v total="$completed" 'BEGIN {position=int(total*0.95+0.999); print (position > 0 ? position : 1)}')
p95_seconds=$(sort -k2,2n "$results" | sed -n "${p95_position}p" | awk '{print $2}')
p95_ms=$(awk -v seconds="${p95_seconds:-10}" 'BEGIN {printf "%.0f", seconds*1000}')

echo "Load test: url=$url requests=$completed concurrency=$concurrency failed=$failed failure_rate=${failure_percent}% p95=${p95_ms}ms"
if awk -v actual="$failure_percent" -v maximum="$max_failure_percent" 'BEGIN {exit !(actual > maximum)}'; then
  echo "FAIL: failure rate exceeded ${max_failure_percent}%" >&2
  exit 1
fi
if [ "$p95_ms" -gt "$max_p95_ms" ]; then
  echo "FAIL: p95 latency exceeded ${max_p95_ms}ms" >&2
  exit 1
fi
echo "PASS: basic production load threshold"
