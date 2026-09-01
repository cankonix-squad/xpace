#!/bin/sh
set -eu

umask 077
root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
archive=${1:-}
passphrase_file=${XPACE_BACKUP_PASSPHRASE_FILE:-}
if [ ! -f "$archive" ] || [ ! -f "$archive.sha256" ] || [ ! -s "$passphrase_file" ]; then
  echo "Usage: XPACE_BACKUP_PASSPHRASE_FILE=/secure/passphrase scripts/restore-test.sh /absolute/xpace-backup.tar.gz.gpg"
  exit 1
fi
case "$archive" in /*) ;; *) echo "FAIL: backup archive path must be absolute"; exit 1;; esac

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/xpace-restore-test.XXXXXX")
test_db="xpace_restore_test_$(date -u +%Y%m%d%H%M%S)_$$"
database_created=0
cleanup() {
  if [ "$database_created" -eq 1 ]; then
    cd "$root_dir"
    docker compose exec -T postgres sh -c 'dropdb -U "$POSTGRES_USER" --if-exists "$1"' sh "$test_db" >/dev/null 2>&1 || true
  fi
  rm -rf -- "$work_dir"
}
trap cleanup EXIT INT TERM

(cd "$(dirname -- "$archive")" && sha256sum -c "$(basename -- "$archive").sha256")
gpg --batch --yes --pinentry-mode loopback --passphrase-file "$passphrase_file" --decrypt --output "$work_dir/backup.tar.gz" "$archive"
mkdir "$work_dir/content"
tar -C "$work_dir/content" -xzf "$work_dir/backup.tar.gz"
(cd "$work_dir/content" && sha256sum -c manifest.sha256)

cd "$root_dir"
docker compose exec -T postgres sh -c 'createdb -U "$POSTGRES_USER" "$1"' sh "$test_db"
database_created=1
docker compose exec -T postgres sh -c 'pg_restore -U "$POSTGRES_USER" -d "$1" --no-owner --no-privileges --exit-on-error' sh "$test_db" < "$work_dir/content/postgres.dump"
validation=$(docker compose exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d "$1" -Atc "SELECT (SELECT COUNT(*) FROM tenants)||'"'"':'"'"'||(SELECT COUNT(*) FROM users)||'"'"':'"'"'||(SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='"'"'public'"'"');"' sh "$test_db")

echo "PASS: encrypted archive, manifest, PostgreSQL restore, and schema validation succeeded"
echo "validation=$validation"
