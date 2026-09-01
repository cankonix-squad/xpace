#!/bin/sh
set -eu

web_url=${XPACE_MONITOR_WEB_URL:-https://${XPACE_DOMAIN:-xspace.cankonix.com}}
api_url=${XPACE_MONITOR_API_URL:-http://127.0.0.1:8080}
livekit_url=${XPACE_MONITOR_LIVEKIT_URL:-https://${LIVEKIT_DOMAIN:-livekit.xspace.cankonix.com}}
prometheus_url=${XPACE_MONITOR_PROMETHEUS_URL:-http://127.0.0.1:9090}
grafana_url=${XPACE_MONITOR_GRAFANA_URL:-http://127.0.0.1:3001}
alertmanager_url=${XPACE_MONITOR_ALERTMANAGER_URL:-http://127.0.0.1:9093}
state_dir=${XPACE_MONITOR_STATE_DIR:-/var/lib/xspace-monitor}
failure_threshold=${XPACE_MONITOR_FAILURE_THRESHOLD:-3}
cooldown_seconds=${XPACE_MONITOR_COOLDOWN_SECONDS:-1800}
alert_recipient=${XPACE_ALERT_EMAIL:-info@cankonix.com}
dry_run=${XPACE_MONITOR_DRY_RUN:-0}

case "$failure_threshold:$cooldown_seconds" in
  *[!0-9:]*|:*) echo "invalid monitoring threshold configuration" >&2; exit 2 ;;
esac

if [ "$dry_run" = "1" ]; then
  state_dir=${XPACE_MONITOR_STATE_DIR:-/tmp/xspace-monitor-dry-run}
fi
mkdir -p "$state_dir"
state_file="$state_dir/state"

previous_failures=0
previous_status=healthy
last_alert_at=0
if [ -f "$state_file" ]; then
  previous_failures=$(sed -n '1p' "$state_file")
  previous_status=$(sed -n '2p' "$state_file")
  last_alert_at=$(sed -n '3p' "$state_file")
fi
case "$previous_failures:$last_alert_at" in *[!0-9:]*) previous_failures=0; last_alert_at=0 ;; esac

failures=""
check_http() {
  label=$1
  url=$2
  expected=$3
  body=$(mktemp)
  code=$(curl --connect-timeout 5 --max-time 15 -sS -o "$body" -w '%{http_code}' "$url" 2>/dev/null || true)
  if [ "$code" != "$expected" ]; then
    failures="${failures}${label} returned HTTP ${code:-000}; "
  fi
  rm -f "$body"
}

check_content() {
  label=$1
  url=$2
  pattern=$3
  body=$(mktemp)
  if ! curl --connect-timeout 5 --max-time 15 -fsS "$url" -o "$body" 2>/dev/null || ! grep -Fq "$pattern" "$body"; then
    failures="${failures}${label} content check failed; "
  fi
  rm -f "$body"
}

send_email() {
  subject=$1
  message=$2
  if [ "$dry_run" = "1" ]; then
    echo "DRY RUN ALERT: $subject — $message"
    return 0
  fi
  if [ -z "${SMTP_HOST:-}" ] || [ -z "${SMTP_PORT:-}" ] || [ -z "${SMTP_FROM:-}" ]; then
    echo "ALERT DELIVERY FAILED: SMTP_HOST, SMTP_PORT, and SMTP_FROM are required" >&2
    return 1
  fi
  envelope_from=$(printf '%s' "$SMTP_FROM" | sed -n 's/.*<\([^>]*\)>.*/\1/p')
  [ -n "$envelope_from" ] || envelope_from=$SMTP_FROM
  smtp_scheme=smtp
  [ "$SMTP_PORT" = "465" ] && smtp_scheme=smtps
  printf 'From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n' "$SMTP_FROM" "$alert_recipient" "$subject" "$message" |
    curl --connect-timeout 10 --max-time 30 -fsS --ssl-reqd \
      --url "$smtp_scheme://$SMTP_HOST:$SMTP_PORT" \
      ${SMTP_USERNAME:+--user "$SMTP_USERNAME:${SMTP_PASSWORD:-}"} \
      --mail-from "$envelope_from" --mail-rcpt "$alert_recipient" --upload-file - >/dev/null
}

check_http "public web" "$web_url/" 200
check_content "API readiness" "$api_url/api/v1/health" '"status":"ready"'
check_content "Prometheus metrics" "$api_url/metrics" 'xpace_api_http_requests_total'
check_content "Prometheus server" "$prometheus_url/-/healthy" 'Prometheus Server is Healthy'
check_content "Prometheus target" "$prometheus_url/api/v1/query?query=up%7Bjob%3D%22xspace-api%22%7D%3D%3D1" '"result":[{"metric"'
check_content "Prometheus rules" "$prometheus_url/api/v1/rules" 'XspaceAPIDown'
check_content "Alertmanager" "$alertmanager_url/-/ready" 'OK'
check_content "Grafana" "$grafana_url/api/health" '"database": "ok"'
check_http "LiveKit TLS route" "$livekit_url/" 200

now=$(date +%s)
if [ -z "$failures" ]; then
  if [ "$previous_status" = "firing" ]; then
    send_email "[RECOVERED] Xspace production" "All monitored Xspace endpoints recovered at $(date -u '+%Y-%m-%d %H:%M:%S UTC')." || true
  fi
  printf '0\nhealthy\n%s\n' "$last_alert_at" > "$state_file"
  echo "PASS: Xspace production endpoints are healthy"
  exit 0
fi

current_failures=$((previous_failures + 1))
status=pending
if [ "$current_failures" -ge "$failure_threshold" ]; then
  status=firing
  if [ $((now - last_alert_at)) -ge "$cooldown_seconds" ]; then
    send_email "[ALERT] Xspace production health" "Health checks failed ${current_failures} consecutive times at $(date -u '+%Y-%m-%d %H:%M:%S UTC'): $failures" || true
    last_alert_at=$now
  fi
fi
printf '%s\n%s\n%s\n' "$current_failures" "$status" "$last_alert_at" > "$state_file"
echo "FAIL: $failures(consecutive failures: $current_failures)" >&2
exit 1
