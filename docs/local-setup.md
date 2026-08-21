# Xpace local setup

## Prerequisites

- Docker Desktop with Docker Compose v2
- Node.js 20 or newer and npm
- Go 1.26 or newer for host-side API tests
- `curl` for health checks

## First start

1. Copy `.env.example` to `.env`.
2. Replace every `replace-with-*` value. Keep `.env` local and never commit it.
3. Ensure ports `3300`, `8080`, `5432`, `6379`, `7880-7882`, `9000-9001`,
   `3478-3479`, and UDP `49160-49200` are available.
4. Start the backend stack:

   ```bash
   docker compose up -d --build
   docker compose ps
   curl -fsS http://127.0.0.1:8080/api/v1/health
   ```

5. Install and start the web application:

   ```bash
   cd apps/web
   npm install
   npm run dev
   ```

6. Open `http://localhost:3300`.

PostgreSQL runs migrations from `services/api/migrations` only when its data
volume is initialized. Apply later migrations deliberately to an existing
database; do not delete a populated volume merely to rerun initialization.

## Bootstrap a workspace

Bootstrap is intended for the first workspace only:

```bash
curl -X POST http://127.0.0.1:8080/api/v1/auth/bootstrap \
  -H 'Content-Type: application/json' \
  -d '{"tenantName":"Cankonix Technology","tenantSlug":"cankonix","displayName":"Admin User","email":"admin@example.com","username":"admin","password":"choose-a-strong-unique-password"}'
```

Sign in at `http://localhost:3300/login` using the workspace slug, username or
email, and the password supplied above.

## Verification

```bash
cd apps/web && npm run build
cd ../../services/api && GOCACHE=/tmp/xpace-go-build-cache go test ./...
cd ../..
scripts/integration-test.sh
scripts/validate-realtime-layers.sh
scripts/validate-audio-priority.sh
scripts/audit-responsive.sh
scripts/audit-accessibility.sh
```

The responsive and accessibility scripts are deterministic source/route
checks. Complete visual viewport, keyboard, and contrast QA in a real browser
before a release.

## Common problems

- Port `3300` unavailable: stop the process using it, then rerun `npm run dev`.
- API not ready: inspect `docker compose logs --tail=100 api postgres`.
- Realtime connection fails: verify LiveKit is healthy and `LIVEKIT_WS_URL` is
  reachable by the browser.
- Camera or microphone denied: allow localhost media access in browser site
  settings and reload prejoin.
- Recording fails: verify `egress`, `minio`, and `minio-init` state with
  `docker compose ps` and inspect their logs.

Stop services with `docker compose down`. This preserves named volumes. Do not
add `-v` unless deleting all local PostgreSQL, Redis, and MinIO data is intended.
