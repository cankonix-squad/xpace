# Xspace production observability

Xspace runs Prometheus and Grafana on loopback-only ports. They are intentionally not exposed through Traefik.

## Start or update

```sh
docker compose -f compose.yaml -f compose.edge.yaml -f compose.observability.yaml up -d
```

Prometheus is available to an operator on server port `127.0.0.1:9090`; Grafana uses `127.0.0.1:3001`. Access either UI through an SSH tunnel rather than a public route:

```sh
ssh -L 3001:127.0.0.1:3001 -L 9090:127.0.0.1:9090 cankonix-hunter
```

Then open `http://127.0.0.1:3001`. The admin user defaults to `admin`. Set `GRAFANA_ADMIN_PASSWORD` in production; when omitted, Compose falls back to the existing API signing key so no default public credential is introduced.

## Retention and alert thresholds

Prometheus retains 30 days or 10 GB by default, whichever limit is reached first. Override these with `PROMETHEUS_RETENTION` and `PROMETHEUS_RETENTION_SIZE`.

Alert rules are stored in `infra/observability/alerts.yml`. Rules cover API availability, telemetry query failure, 5xx ratio, p95 latency, database pool pressure, recording failures, waiting-room backlog, and stored-content capacity. Prometheus evaluates these rules locally and sends firing/resolved events to the private Alertmanager.

## Acceptance checks

```sh
curl -fsS http://127.0.0.1:9090/-/healthy
curl -fsS http://127.0.0.1:9090/api/v1/targets
curl -fsS http://127.0.0.1:9090/api/v1/rules
curl -fsS http://127.0.0.1:3001/api/health
```

The existing `xspace-monitor.timer` also checks Prometheus health, the Xspace API target, loaded alert rules, and Grafana database health every minute. It uses the existing consecutive-failure threshold and SMTP recovery/alert delivery.

Prometheus sends firing and resolved alerts to the private Alertmanager on `127.0.0.1:9093`. Alertmanager groups notifications by alert, severity, and subsystem, then delivers them through the production SMTP account to `XPACE_ALERT_EMAIL` (default `info@cankonix.com`). The SMTP password is written to a mode-`0600` ephemeral runtime file and is not embedded in the Alertmanager configuration.

The same receiver posts to the bearer-authenticated internal API webhook. Xspace resolves the operations workspace through `XSPACE_OPERATIONS_TENANT_SLUG`, deduplicates cases by Alertmanager fingerprint, maps critical/warning/info to P1/P2/P4 (P3 fallback), records firing/resolved timeline events, and writes system audit entries. `ALERTMANAGER_WEBHOOK_SECRET` should be an independent 32-byte secret; deployments without it temporarily fall back to the API signing key for backward-compatible rollout.

## Incident acknowledgement escalation

The API runs one advisory-lock-protected escalation worker across all replicas. An open, unacknowledged incident is escalated after:

- P1: 5 minutes
- P2: 15 minutes
- P3: 1 hour
- P4: 4 hours

Each escalation is one atomic database transaction: the incident receives escalation level 1 and `last_escalated_at`, an `ESCALATED` timeline event and immutable `incident.escalate` audit entry are created, active tenant/super admins receive an internal notification, and one deduplicated email per administrator is queued. If any write fails, the transaction rolls back so the worker can retry safely.

The worker is controlled by `INCIDENT_ESCALATION_WORKER_ENABLED`, `INCIDENT_ESCALATION_INITIAL_DELAY`, and `INCIDENT_ESCALATION_INTERVAL`. Production defaults are enabled, one-minute initial delay, and one-minute polling. A controlled real-email drill still requires approval from the operational mailbox recipient.
