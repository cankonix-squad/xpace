# Xpace PoC runbook

## Purpose

This runbook prepares, operates, validates, and recovers the Xpace proof of
concept. It assumes the local Compose deployment; production TLS differences
are noted separately.

## Start and readiness

```bash
docker compose up -d --build
docker compose ps
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/api/v1/health
curl -fsSI http://127.0.0.1:3300/
```

Expected state: PostgreSQL, Redis, MinIO, LiveKit, API, coturn, and Egress are
running; `minio-init` has exited successfully; API readiness reports PostgreSQL
as `ok`.

## Pre-demo gate

```bash
scripts/deployment-preflight.sh
scripts/poc-smoke.sh
scripts/integration-test.sh
scripts/validate-realtime-layers.sh
scripts/validate-audio-priority.sh
scripts/audit-responsive.sh
scripts/audit-accessibility.sh
docker compose run --rm --entrypoint /usr/local/bin/recording-smoke api
```

For TLS deployment, point the same smoke test at public endpoints:

```bash
XPACE_WEB_URL=https://xspace.cankonix.com \
XPACE_API_URL=https://xspace.cankonix.com \
XPACE_LIVEKIT_URL=https://livekit.xspace.cankonix.com \
XPACE_RECORDING_URL=skip \
scripts/poc-smoke.sh
```

Confirm that the MinIO `xpace-recordings` bucket remains private and that no
test reports secrets, object keys, or raw session tokens.

## Operator checks

- Sign in and open `/admin`; confirm API and PostgreSQL health.
- Create a meeting and copy its join code.
- Join with a second account; confirm waiting-room admission.
- Confirm mic/camera selection in prejoin and a successful LiveKit connection.
- Exercise lock, participant mute/promote/remove, and end-for-all.
- Start and stop a short recording; confirm authorized download issuance.
- Review `/admin/audit` for authentication and moderation events.

## Incident triage

```bash
docker compose ps
docker compose logs --tail=150 api livekit egress coturn
curl -fsS http://127.0.0.1:8080/api/v1/health
```

- API/database failure: preserve logs and data volumes; restart only the failed
  service with `docker compose restart <service>`.
- Signaling failure: check LiveKit health, Redis authentication, and WebSocket
  URL before investigating TURN.
- Media path failure: confirm TCP/UDP firewall ports and test another network.
- Recording failure: inspect Egress and MinIO first; do not make the bucket
  public as a workaround.

## TLS deployment

Set DNS for `xspace.cankonix.com` and `livekit.xspace.cankonix.com`, configure
`ACME_EMAIL`, allow the documented media ports, then run:

```bash
docker compose -f compose.yaml -f compose.proxy.yaml up -d --build
```

Validate public DNS and trusted TLS independently of the application smoke
test:

```bash
scripts/production-readiness.sh
```

If the certificate subject is `TRAEFIK DEFAULT CERT`, the HTTPS entrypoint is
reachable but the hostname router has no issued certificate. Confirm the
deployed `XPACE_DOMAIN`, set `ACME_EMAIL`, inspect Traefik ACME logs/state, and
rerun the readiness check. Never bypass this gate with insecure TLS flags.

Verify HTTPS redirect, certificate validity, secure cookies, WebSocket
signaling, API readiness, and an external-network meeting before announcing the
PoC. Roll back by restoring the previous image/configuration; preserve database
and object-storage volumes.

When the host already operates the shared `cankonix-traefik` edge, use the
external `cankonix-proxy` network instead of starting a second proxy:

```bash
docker compose -f compose.yaml -f compose.edge.yaml up -d --build
```
