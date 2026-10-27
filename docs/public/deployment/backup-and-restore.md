# Backup, restore and upgrades

Keep the PostgreSQL database, trusted tenant/identity configuration, external rule files and referenced secrets recoverable together.

- The database contains tenant resources, measurements, workflow/replay state, identity sessions, authorization data, decision journals and archive delivery work/receipts.
- Backups can contain sensitive tenant and replay data; store them with restricted access and encryption under your normal retention policy.
- The MVP provides no automatic backup service or reversible migration framework. Do not assume a previous binary can read a database after a newer binary initializes it.

## Logical backup with Compose

- These commands use the explicit `nines` Compose project from the [container guide](docker-compose.md). They stop collection while taking a logical backup, then restart the same instance:

```sh
mkdir -p backups
chmod 700 backups
umask 077
docker compose -p nines stop platform
docker compose -p nines exec -T database pg_dump -U platform -d platform --format=custom > backups/nines.dump
docker compose -p nines start platform
```

- Check that `pg_dump` succeeded before treating the file as a backup.
  - If it fails, preserve the error and restart the server deliberately.
  - Keep all tenant schemas and identity tables; do not back up only the default tenant.
  - PostgreSQL's consistent dump covers committed database state, while stopped collection avoids advancing application work during the maintenance window.
- Back up `.env`, mounted tenant/auth YAML, `NINES_SOURCE_ENCRYPTION_KEY`, private CA files, rule destinations and secret-manager version references separately.
  - Managed source credentials cannot be decrypted after restore without the original encryption key.
  - Protect those copies at least as carefully as the database.
  - PostgreSQL role definitions are managed outside a normal database dump; recreate the expected role and grants when restoring.

## Rehearse restore in an isolated database

- Before starting a restored Nines server for a rehearsal, remove `auditArchive` from the rehearsal tenant configuration or point it to a separately provisioned acceptance destination.
  - A restored queue can resume external writes.
  - An existing immutable archive is not rewound by restoring PostgreSQL; inspect post-backup decisions and their external versions before resuming production changes. See [audit archival](audit-archive.md).

Use a different Compose project so the rehearsal has a new database volume. Start only its database, then restore into that fresh database:

```sh
docker compose -p nines-restore up -d database
# Wait for its database health check before restoring.
docker compose -p nines-restore exec -T database pg_isready -U platform -d platform
docker compose -p nines-restore exec -T database pg_restore --exit-on-error --no-owner --no-acl -U platform -d platform < backups/nines.dump
```

- Do not restore over the original database or use `--clean` against an instance you have not explicitly chosen to replace.
  - Inspect the archive's table/schema inventory and restore logs.
  - Verify representative resources, retained measurements, tenant boundaries, memberships and audit state.
- Before starting any restored Nines process, isolate it from production providers, webhooks, model services and rule destinations.
  - Recreate trusted tenant IDs and configuration deliberately and use dedicated test credentials.
  - Do not blindly start a second collecting server with copied production integrations.
  - Persisted workflow and alert state can resume after startup.
- The local deployment proof exercises a disposable database backup/restore round trip.
  - It does not establish your encryption policy, point-in-time recovery, off-site replication, infrastructure recovery or successful restoration of a real deployment.
  - Record a dated restore exercise for your own environment.

## Upgrade or roll back

1. Record the deployed source commit/image digest and retain its binary/image.
2. Stop the single collecting server and create a verified database/configuration backup.
3. Start the reviewed replacement, inspect startup logs and verify authenticated access to each tenant.
4. Check query freshness, rule destinations and alert routing before ending maintenance.
5. If rollback is needed, verify schema compatibility first. Where compatibility is unknown, restore the pre-upgrade database into an isolated replacement and recover deliberately.

- Do not run old and new collecting processes simultaneously. The normal [Kubernetes reference](kubernetes.md) uses `Recreate`, and the native service upgrade replaces one stopped process.
- Reference: PostgreSQL's [pg_dump](https://www.postgresql.org/docs/17/app-pgdump.html) and [pg_restore](https://www.postgresql.org/docs/17/app-pgrestore.html) documentation.
