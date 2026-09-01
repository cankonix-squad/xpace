#!/bin/sh
set -eu

umask 077
root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
target_dir=${1:-}
passphrase_file=${XPACE_BACKUP_PASSPHRASE_FILE:-}

if [ -z "$target_dir" ] || [ "${target_dir#/}" = "$target_dir" ] || [ "$target_dir" = "/" ] || [ "$target_dir" = "${HOME:-/nonexistent}" ]; then
  echo "Usage: XPACE_BACKUP_PASSPHRASE_FILE=/secure/passphrase scripts/backup.sh /absolute/backup-directory"
  exit 1
fi
if [ ! -f "$passphrase_file" ] || [ ! -s "$passphrase_file" ]; then
  echo "FAIL: XPACE_BACKUP_PASSPHRASE_FILE must reference a non-empty protected file"
  exit 1
fi
for command_name in docker gpg tar sha256sum mktemp; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "FAIL: required command is unavailable: $command_name"; exit 1; }
done

mkdir -p "$target_dir"
target_dir=$(CDPATH= cd -- "$target_dir" && pwd)
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
work_dir=$(mktemp -d "$target_dir/.xpace-backup-work.XXXXXX")
plain_archive="$target_dir/.xpace-$timestamp.tar.gz"
final_archive="$target_dir/xpace-$timestamp.tar.gz.gpg"
cleanup() {
  rm -rf -- "$work_dir"
  rm -f -- "$plain_archive"
}
trap cleanup EXIT INT TERM

cd "$root_dir"
docker compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$work_dir/postgres.dump"
test -s "$work_dir/postgres.dump" || { echo "FAIL: PostgreSQL backup is empty"; exit 1; }

mkdir "$work_dir/objects"
docker compose run --rm -T --no-deps -v "$work_dir/objects:/backup/objects" --entrypoint /bin/sh minio-init -c 'mc alias set local http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null && mc mirror --overwrite local/xpace-recordings /backup/objects' >/dev/null

{
  echo "format_version=1"
  echo "created_at=$timestamp"
  echo "database=postgres.dump"
  echo "objects=objects/"
  echo "encryption=gpg-aes256"
} > "$work_dir/backup.meta"
(cd "$work_dir" && find . -type f ! -name manifest.sha256 -print0 | sort -z | xargs -0 sha256sum) > "$work_dir/manifest.sha256"
tar -C "$work_dir" -czf "$plain_archive" .
gpg --batch --yes --pinentry-mode loopback --passphrase-file "$passphrase_file" --symmetric --cipher-algo AES256 --output "$final_archive" "$plain_archive"
sha256sum "$final_archive" > "$final_archive.sha256"

echo "PASS: encrypted Xpace backup created"
echo "$final_archive"
