#!/bin/sh
set -eu

repository_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$repository_root"

set -a
. ./.env
set +a

export DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@127.0.0.1:5432/${POSTGRES_DB}?sslmode=disable"
export XPACE_TEST_API_URL="${XPACE_TEST_API_URL:-http://127.0.0.1:8080}"
export GOCACHE="${GOCACHE:-/tmp/xpace-go-build-cache}"

cd services/api
go test -tags=integration ./internal/httpapi -run '^TestIntegration(AuthMeetingLifecycle|TenantIsolation)$' -count=1 -v
