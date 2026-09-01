# CANKONIX Xpace

Official repository: `https://github.com/cankonix-squad/xpace`

Production application domain: `https://xspace.cankonix.com`

Secure adaptive digital workspace and collaboration platform.

Operational documentation:

- [Local setup](docs/local-setup.md)
- [PoC runbook](docs/poc-runbook.md)
- [PoC demo script](docs/demo-script.md)
- [Backup and disaster recovery](docs/disaster-recovery.md)

## Workspace

- `apps/web` — Next.js, React, TypeScript, Tailwind CSS
- `services/api` — Go API (health endpoint and domain structure foundation)
- `tasklist.md` — the live project checklist; completed items use `- [x]`

## Local development

### Data services

Copy `.env.example` to `.env`, replace every placeholder with a local secret,
then start PostgreSQL, Redis, and MinIO:

```bash
docker compose up -d postgres redis minio
```

The data services are bound to `127.0.0.1` only. The initial schema is applied by
PostgreSQL from `services/api/migrations/0001_initial.sql` on its first start.

### Realtime media

Set `LIVEKIT_API_KEY`, `LIVEKIT_API_SECRET`, `LIVEKIT_WS_URL`,
`COTURN_AUTH_SECRET`, and `COTURN_REALM` in `.env`, then start the realtime
services:

```bash
docker compose up -d redis livekit coturn
```

Local ports are reserved as follows:

- Xpace Web: `3300/tcp`
- Xpace API: `8080/tcp`
- LiveKit signaling: `7880/tcp`
- LiveKit WebRTC: `7881/tcp`, `7882/udp`
- LiveKit embedded TURN: `3479/udp`
- coturn: `3478/tcp+udp`, `49160-49200/udp`

The API issues short-lived LiveKit tokens only after an authenticated user has
completed the join flow. Participants in the waiting room do not receive media
tokens until admitted by the host.

Inside the realtime room, the host participant panel polls the secured meeting
API for waiting and joined participants. Host-only actions support admit,
reject, promote to co-host, and remove; each moderation action is recorded in
the tenant audit log.

Realtime moderation uses the official LiveKit Room Service SDK. Server-side
operations support force-muting an active audio track, disconnecting a removed
participant, updating co-host permissions, locking/unlocking new joins, and
ending the room for everyone. The host room header exposes lock and end-room
controls; co-hosts inherit participant-panel access after promotion.

Participant lifecycle is synchronized back to PostgreSQL when the LiveKit room
disconnects. The room UI shows a dedicated meeting-ended state, and the host
participant panel exposes force mute alongside promote and remove actions.

The realtime room surfaces LiveKit connection quality with text and signal bars,
local publish bitrate and resolution, and an explicit reconnect overlay. Users
can enable low-bandwidth mode to pause video and prioritize audio, then restore
video at a conservative 640×360 / 15 FPS profile.

The network diagnostics panel reads browser `RTCStatsReport` data directly and
shows RTT, jitter, packet loss, outbound bitrate, FPS, capture resolution,
codec, and simulcast RID/layer when reported by the browser. Adaptive mode
downgrades publishing to the low layer after three consecutive poor-quality
samples, recovers through the medium layer after three excellent samples, and
restores the high layer after six stable samples. It can be disabled by the
user. A deterministic in-room demo runs the complete normal → limited → poor
→ recovery sequence without requiring operating-system network shaping.

The room explicitly enables LiveKit adaptive stream and dynacast. Camera
publishing requests simulcast with up to three spatial layers; SVC-capable
codecs use `L3T3_KEY`. Audio publishing enables DTX and RED so silence consumes
less bandwidth while redundant packets improve recovery on lossy networks.

Validate the checked-in room options against the installed LiveKit SDK and the
running local server:

```bash
scripts/validate-realtime-layers.sh
```

Validate the low-bandwidth audio-priority contract, including video pause,
DTX/RED audio protection, and conservative camera recovery:

```bash
scripts/validate-audio-priority.sh
```

Hosts can start and stop a composite MP4 recording from the room header. A
dedicated LiveKit Egress worker renders the room, stores the file under a
tenant/meeting-scoped object key in the private `xpace-recordings` MinIO bucket,
and persists recording lifecycle metadata and audit events in PostgreSQL. Only
the meeting host can control recording; listing remains tenant-session scoped.

### Web

```bash
cd apps/web
npm run dev
```

Xpace Web always uses `http://localhost:3300` in both development and
production. Port `3300` is reserved for this web application and must not be
assigned to another Xpace service.

With the local web server running, audit responsive breakpoints, critical
routes, and browser media permissions:

```bash
scripts/audit-responsive.sh
```

Run the deterministic accessibility source audit for keyboard focus, control
names, semantic alerts/dialogs, reduced motion, and non-color status text:

```bash
scripts/audit-accessibility.sh
```

### API

```bash
cd services/api
go run ./cmd/api
```

The API is also packaged as a non-root container and is started with the rest
of the local stack:

```bash
docker compose up -d --build api egress
```

Run the repeatable recording pipeline smoke test from the Compose network:

```bash
docker compose run --rm --entrypoint /usr/local/bin/recording-smoke api
```

The command joins a temporary LiveKit room, publishes a synthetic Opus audio
track, records it through Egress, uploads an MP4 under `smoke-tests/`, then
removes the temporary room. The bucket remains private.


The API listens on `http://localhost:8080` by default and exposes
`GET /healthz` (liveness), `GET /api/v1/health` (PostgreSQL readiness), and
`GET /metrics` (Prometheus exposition). Every API request emits a structured
JSON log with method, normalized route, status, duration, and remote address.
The API and production web containers include Docker health checks; Traefik
access logs provide edge-level request visibility.

Production operations add a one-minute independent endpoint monitor with
consecutive-failure suppression and SMTP recovery/incident alerts. Install or
refresh its systemd timer from the production project directory, then run the
read-only baseline load check:

```bash
scripts/install-production-monitor.sh
XPACE_LOAD_URL=http://127.0.0.1:8080/healthz scripts/load-test-basic.sh
```

The default baseline is 200 requests at concurrency 10, with at most 1% failed
requests and p95 latency no higher than 1000 ms. Operational ownership,
severity, escalation, recovery, and customer-support handling are documented
in `docs/incident-response.md`.

### TLS-ready deployment

The production override adds a non-root Next.js container and Traefik reverse
proxy. It serves the application and `/api/v1` from `XPACE_DOMAIN`, routes
LiveKit WebSocket signaling through `LIVEKIT_DOMAIN`, redirects HTTP to HTTPS,
and obtains certificates from Let's Encrypt.

Create DNS A/AAAA records for both domains, allow inbound TCP 80/443 plus the
documented WebRTC/TURN ports, set `XPACE_DOMAIN`, `LIVEKIT_DOMAIN`, and
`ACME_EMAIL` in `.env`, then run:

```bash
docker compose -f compose.yaml -f compose.proxy.yaml up -d --build
```

Run preflight before changing deployment state and smoke test after startup:

```bash
scripts/deployment-preflight.sh
scripts/poc-smoke.sh
scripts/production-readiness.sh
```

Traefik stores ACME state in the `letsencrypt-data` volume. The Docker socket is
mounted read-only and only explicitly labelled services are published. Keep the
API, database, Redis, and MinIO host bindings restricted to loopback or remove
those host bindings in a hardened production environment.

### First workspace and authentication

Initialize the first workspace once:

```bash
curl -X POST http://localhost:8080/api/v1/auth/bootstrap \
  -H 'Content-Type: application/json' \
  -d '{"tenantName":"Cankonix Technology","tenantSlug":"cankonix","displayName":"Admin User","email":"admin@cankonix.id","username":"admin","password":"choose-a-strong-unique-password"}'
```

The web app proxies `/api/v1/*` to `API_URL` and provides the sign-in screen at
`http://localhost:3300/login`. Authentication uses a random, hashed server-side
session with an HttpOnly SameSite cookie. Set `COOKIE_SECURE=true` behind HTTPS.

Authenticated user payloads include a role and explicit permissions. Workspace
authority uses `SUPER_ADMIN` and `TENANT_ADMIN`; meeting participation supports
`HOST`, `CO_HOST`, `MEMBER`, and `GUEST`. Host/co-host authority remains scoped
to its meeting, while workspace admins can moderate meetings in their tenant.
Guests can join meetings but cannot create them.

User profiles are available through `GET|PATCH /api/v1/profile` and store the
display name, IANA timezone, locale, short bio, and optional avatar metadata.
Tenant identity is included in the authenticated session payload. Composite
database constraints prevent meetings, participants, recordings, profiles, and
audit actors from referencing users or meetings in another tenant.

Authenticated meeting endpoints:

- `GET|POST /api/v1/meetings`
- `GET /api/v1/meetings/history?limit=25&offset=0`
- `GET /api/v1/meetings/{joinCode}`
- `POST /api/v1/meetings/{joinCode}/join`
- `GET|POST /api/v1/meetings/{joinCode}/recordings`
- `POST /api/v1/meetings/{joinCode}/recordings/stop`
- `PUT|DELETE /api/v1/meetings/{joinCode}/recordings/{recordingID}/access/{userID}`
- `GET /api/v1/meetings/{joinCode}/recordings/{recordingID}/download`

Workspace administrators can open `/admin` or call
`GET /api/v1/admin/dashboard` for tenant-scoped meeting, participant, user,
recording usage, seven-day activity, recent meetings, and API/PostgreSQL health.
The `/admin/users` page and `GET|POST /api/v1/admin/users` plus
`PATCH /api/v1/admin/users/{userID}` endpoints provide tenant-scoped account,
role, invitation, suspension, and deactivation management with audit events.
The `/admin/groups` page supports tenant-scoped group creation, deletion, and
membership management through `/api/v1/admin/groups` endpoints.
The `/admin/meetings` page provides searchable, status-filtered meeting lists
and per-meeting participant, duration, and recording analytics through
`GET /api/v1/admin/meetings` and `GET /api/v1/admin/meetings/{meetingID}`.
Tenant administrators can review filterable, paginated authentication,
administration, recording, and moderation events at `/admin/audit`, backed by
`GET /api/v1/admin/audit-events`.
Workspace meeting defaults are managed at `/admin/policies` through
`GET|PUT /api/v1/admin/meeting-policy`. Guest admission, waiting-room default,
recording, and screen-share publishing are enforced server-side.

Workspace administrators can manage tenant-scoped system defaults at
`http://localhost:3300/admin/settings` through
`GET|PUT /api/v1/admin/system-configuration`. Settings include workspace name,
timezone, locale, support email, meeting duration, and recording retention.

Plan, entitlement, quota usage, invoice history, and scheduled cancellation are
available at `/admin/billing`. Billing adapters post normalized lifecycle events
to `POST /api/v1/billing/webhooks/{provider}` with an
`X-Xpace-Signature: sha256=<hex-hmac>` header computed over the exact request
body. Set a long random `BILLING_WEBHOOK_SECRET` before enabling a provider; the
endpoint remains unavailable when the secret is empty. Webhook event IDs are
idempotent and cannot be reused with a different payload.

The first native provider adapter uses Xendit Payment Sessions API version
`2026-01-01`. Configure `XENDIT_SECRET_KEY` and `XENDIT_WEBHOOK_TOKEN`, then set
the Xendit Payment Session and Subscription webhook URL to
`https://xspace.cankonix.com/api/v1/billing/webhooks/xendit/native`. The
server-side checkout endpoint is `POST /api/v1/admin/billing/checkout`; checkout
controls remain disabled when the Xendit secret key is absent.
Provider-managed cancellation calls Xendit's plan deactivation endpoint before
scheduling local period-end access; an inactive Xendit plan must be restarted
through a new checkout rather than resumed locally.

Meeting history is tenant-scoped and only returns meetings the current user
hosted or joined; workspace admins can review their entire tenant. Each item
includes the user's meeting role, duration, participant count, recording count,
and bounded offset pagination metadata.

Recording metadata is private by default. It is visible only to the meeting
host, recording starter, workspace administrators, or users with an explicit
tenant-scoped access grant. Hosts and workspace admins can grant or revoke
access; object-storage keys are never returned to browsers.
Authorized recording downloads return a five-minute S3-compatible presigned
URL, while unauthorized or cross-tenant requests receive the same not-found
response. Session cookies carry an HMAC-SHA256 signature and support controlled
key rotation through `API_SESSION_SIGNING_KEY_PREVIOUS`; opaque session tokens
remain stored only as SHA-256 hashes. API startup rejects missing, short, or
documented-placeholder runtime secrets.

All authenticated data access is scoped by the tenant resolved from the
server-side session. Administrative endpoints require `SUPER_ADMIN` or
`TENANT_ADMIN`; meeting creation and moderation use explicit role and ownership
checks. Participant, LiveKit token, lifecycle, moderation, and recording
queries additionally carry `tenant_id` predicates as defense in depth.

The API applies IP-based fixed-window limits (stricter for login/bootstrap),
64 KiB JSON body limits, unknown-field rejection, safe panic recovery, and
non-reflective parser errors. API and web responses include CSP, clickjacking,
MIME-sniffing, referrer, resource, and browser-permission protections; HSTS is
enabled for HTTPS API traffic and production web responses.

Security audit coverage includes successful and failed login, logout, rejected
sessions, meeting join, realtime-token issuance, meeting policy changes,
recording access/download issuance, user and group administration, and all
participant/meeting moderation actions. Attempted login identities are stored
only as short SHA-256-derived correlation hashes; raw passwords, tokens, signed
URLs, and object keys are never written to audit metadata.

Run the backend unit suite, including SQL-mocked handler tests and a coverage
report, with:

```bash
cd services/api
GOCACHE=/tmp/xpace-go-build-cache go test ./... -coverprofile=/tmp/xpace-cover.out
GOCACHE=/tmp/xpace-go-build-cache go tool cover -func=/tmp/xpace-cover.out
```

The suite covers password hashing, signed-session rotation/tamper rejection,
role authorization, login/logout, meeting creation and tenant lookup, audit
persistence, recording configuration, input validation, rate limiting, panic
recovery, health/readiness, pagination, and runtime-secret validation.

Run the real-service authentication and meeting lifecycle integration test
against the local Compose stack with:

```bash
scripts/integration-test.sh
```

The test creates an isolated temporary tenant, exercises two signed-cookie
sessions through create → waiting room → admit → realtime token → leave → end →
history → logout, verifies the corresponding audit events, and removes all test
data even when an assertion fails.

The join flow validates the code before opening
`/meet/{joinCode}/prejoin`, where users can preview and select their camera,
microphone, and speaker. Joining then routes the participant to either the
waiting-room state or the meeting-room handoff for the upcoming LiveKit stage.
