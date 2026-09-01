# Xspace incident and support runbook

## Ownership and contact

- Incident commander: Cankonix Technology operations owner on duty.
- Customer contact: `info@cankonix.com` until a dedicated support address is approved.
- Production alerts: `XPACE_ALERT_EMAIL` (defaults to `info@cankonix.com`).
- Record every incident start/end time, affected workspace or region, customer impact, decisions, and remediation owner.

Never place passwords, session cookies, meeting tokens, SMTP credentials, recording URLs, or personal message content in incident notes.

## Severity and response target

| Severity | Example | Acknowledge | Update cadence |
| --- | --- | ---: | ---: |
| SEV-1 | Login/meeting unavailable for most customers, suspected breach, tenant isolation failure, unrecoverable data risk | 15 minutes | 30 minutes |
| SEV-2 | Major feature unavailable, repeated recording failure, billing lifecycle failure affecting several customers | 30 minutes | 60 minutes |
| SEV-3 | Degraded performance or isolated customer problem with workaround | 4 business hours | Daily |
| SEV-4 | Question, cosmetic defect, or planned service request | 1 business day | As agreed |

Targets are operational goals, not contractual SLAs, until formally published in customer terms.

## Detection and first response

1. Confirm the alert independently using `curl -fsS https://xspace.cankonix.com/healthz` and `docker compose ps`.
2. Declare severity, incident commander, impact, and start time. Open an internal incident record.
3. Preserve relevant structured logs and audit events. Do not delete or rotate evidence during investigation.
4. Stop risky deployments. Prefer rollback of the last isolated change over unrelated modifications.
5. Check API/PostgreSQL/Redis/LiveKit/MinIO health, capacity, certificates, DNS, and recent deployments.
6. For suspected security or tenant-isolation events, revoke affected sessions/credentials, restrict support access, preserve evidence, and notify the business owner immediately.
7. Communicate confirmed facts, current impact, workaround, and next update time. Do not speculate.

## Recovery paths

- Web/API regression: redeploy the last verified image or revert the isolated change, then run health and acceptance smoke tests.
- Database/storage recovery: follow `docs/disaster-recovery.md`; never restore over production without an approved recovery point and backup validation.
- LiveKit/media issue: verify LiveKit, Redis, TURN, DNS/TLS, and cross-network acceptance before declaring recovery.
- SMTP issue: inspect only aggregate outbox state and redacted errors; restore SMTP connectivity and allow retry worker delivery.
- Billing issue: pause customer-facing billing actions if integrity is uncertain; preserve webhook payload hashes and reconcile provider events idempotently.

## Customer support workflow

1. Authenticate the requester and workspace before discussing tenant data.
2. Request the meeting code, timestamp/timezone, affected feature, browser/device, and reproducible steps—not passwords or tokens.
3. Use time-limited read-only platform support access with a recorded reason when cross-workspace inspection is required.
4. Link the case to an incident when impact is shared. Keep tenant-specific details out of public updates.
5. Confirm resolution with the requester and document the workaround or permanent fix.

## Resolution and review

An incident is resolved only after health checks, the affected user journey, and relevant data integrity checks pass. For SEV-1/SEV-2, publish an internal review within five business days covering timeline, root cause, impact, detection gap, corrective actions, owners, and deadlines. Track corrective actions in `tasklist.md`.
