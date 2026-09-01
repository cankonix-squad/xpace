# Xpace Backup and Disaster Recovery

## Recovery objectives

- Paid-beta target RPO: 24 hours until continuous/WAL archiving is introduced.
- Paid-beta target RTO: 2 hours for a single-node deployment with a verified encrypted archive.
- Retain daily backups for 14 days, weekly backups for 8 weeks, and monthly backups for 12 months.
- Copy encrypted archives and checksum files to a different provider or failure domain. A backup stored only on the Xpace host is not considered complete.

## Create an encrypted backup

Create a root-readable passphrase file outside the repository (`chmod 600`). Then run:

```sh
XPACE_BACKUP_PASSPHRASE_FILE=/secure/xpace-backup-passphrase \
  scripts/backup.sh /srv/xpace-backups
```

The archive contains a PostgreSQL custom-format dump, the private MinIO bucket, metadata, and per-file checksums. The outer archive is encrypted with GPG AES-256 and has a separate SHA-256 checksum.

Copy both `.tar.gz.gpg` and `.sha256` files offsite. Keep the passphrase in a separate secret manager; do not store it beside the backup.

## Scheduled operation

Run `scripts/backup.sh` daily using a systemd timer or the infrastructure scheduler. Alert when no successful archive has been uploaded offsite within 26 hours. Run `scripts/restore-test.sh` at least weekly against the latest archive.

## Non-destructive restore test

```sh
XPACE_BACKUP_PASSPHRASE_FILE=/secure/xpace-backup-passphrase \
  scripts/restore-test.sh /srv/xpace-backups/xpace-YYYYMMDDTHHMMSSZ.tar.gz.gpg
```

This verifies the encrypted archive and manifest, restores PostgreSQL into a disposable database, validates tenant/user/schema counts, and drops only that disposable database. It never changes production MinIO or the live database.

## Production restore

1. Declare the incident, record its start time, owner, scope, and chosen recovery point.
2. Preserve the failed system for investigation; never overwrite the only available copy.
3. Verify the archive checksum and confirm that an offsite copy remains available.
4. Notify customers of expected downtime when required.
5. Set the explicit confirmation and run the restore:

```sh
XPACE_BACKUP_PASSPHRASE_FILE=/secure/xpace-backup-passphrase \
XPACE_RESTORE_CONFIRM=RESTORE_PRODUCTION \
  scripts/restore.sh /srv/xpace-backups/xpace-YYYYMMDDTHHMMSSZ.tar.gz.gpg
```

The command stops API/web/egress, restores PostgreSQL with `--clean`, mirrors the backed-up private bucket with `--remove`, restarts services, and performs a database check. This is destructive to current production state and must only be used for an approved recovery.

6. Run health, login, meeting, recording-download, Drive, and governance smoke tests.
7. Record actual RPO/RTO, missing data, validation results, and follow-up actions.

## Remaining high-availability work

The current production topology remains single-region and single-node. High availability requires managed/replicated PostgreSQL, replicated object storage, redundant API/web/media nodes, automated failover, and periodic chaos/failover testing.
