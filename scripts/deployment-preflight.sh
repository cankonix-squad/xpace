#!/bin/sh
set -eu

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root_dir"

for command_name in docker curl; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "FAIL: required command is unavailable: $command_name"
    exit 1
  fi
done
echo "PASS: required deployment commands are available"

if [ ! -f .env ]; then
  echo "FAIL: .env is missing; copy .env.example and set deployment secrets"
  exit 1
fi

if grep -Eq '=replace-with-|=ops@example\.com$' .env; then
  echo "FAIL: .env still contains documented placeholder values"
  exit 1
fi
echo "PASS: .env contains no documented placeholders"

docker compose config -q
echo "PASS: base Compose configuration is valid"
docker compose -f compose.yaml -f compose.proxy.yaml config -q
echo "PASS: TLS Compose configuration is valid"
docker compose -f compose.yaml -f compose.edge.yaml config -q
echo "PASS: shared-edge Compose configuration is valid"

for file in apps/web/Dockerfile services/api/Dockerfile compose.yaml compose.proxy.yaml compose.edge.yaml; do
  test -s "$file" || { echo "FAIL: missing deployment file: $file"; exit 1; }
done
echo "PASS: deployment manifests and images are present"

if ! grep -Fq 'USER nextjs' apps/web/Dockerfile || ! grep -Fq 'USER xpace' services/api/Dockerfile; then
  echo "FAIL: application containers must run as non-root users"
  exit 1
fi
echo "PASS: application runtime images use non-root users"

echo "Deployment preflight passed."
