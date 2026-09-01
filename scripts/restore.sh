#!/bin/sh
set -eu

umask 077
root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
archive=${1:-}
passphrase_file=${XPACE_BACKUP_PASSPHRASE_FILE:-}
if [ "${XPACE_RESTORE_CONFIRM:-}" != "RESTORE_PRODUCTION" ]; then
  echo "FAIL: set XPACE_RESTORE_CONFIRM=RESTORE_PRODUCTION after reviewing the disaster-recovery runbook"
  exit 1
fi
if [ ! -f "$archive" ] || [ ! -f "$archive.sha256" ] || [ ! -s "$passphrase_file" ]; then
  echo "FAIL: archive, checksum, or XPACE_BACKUP_PASSPHRASE_FILE is unavailable"
  exit 1
fi
case "$archive" in /*) ;; *) echo "FAIL: backup archive path must be absolute"; exit 1;; esac

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/xpace-restore.XXXXXX")
services_stopped=0
cleanup() {
  rm -rf -- "$work_dir"
  if [ "$services_stopped" -eq 1 ]; then
    cd "$root_dir"
    docker compose up -d api web egress >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT INT TERM

(cd "$(dirname -- "$archive")" && sha256sum -c "$(basename -- "$archive").sha256")
gpg --batch --yes --pinentry-mode loopback --passphrase-file "$passphrase_file" --decrypt --output "$work_dir/backup.tar.gz" "$archive"
mkdir "$work_dir/content"
tar -C "$work_dir/content" -xzf "$work_dir/backup.tar.gz"
(cd "$work_dir/content" && sha256sum -c manifest.sha256)

cd "$root_dir"
docker compose stop api web egress
services_stopped=1
docker compose exec -T postgres sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --no-privileges --exit-on-error' < "$work_dir/content/postgres.dump"
docker compose run --rm -T --no-deps -v "$work_dir/content/objects:/restore/objects:ro" --entrypoint /bin/sh minio-init -c 'mc alias set local http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null && mc mirror --overwrite --remove /restore/objects local/xpace-recordings' >/dev/null
docker compose up -d api web egress
services_stopped=0
docker compose exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "SELECT COUNT(*) FROM tenants;"' >/dev/null
echo "PASS: Xpace database and private object storage restored; services restarted"
