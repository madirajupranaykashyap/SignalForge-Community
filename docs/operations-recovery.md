# Operations recovery runbook

## Control-plane outage

1. Do not restart the Collector just because the management API is unavailable. The agent keeps the current pipeline running and retries remote polling with bounded exponential backoff and jitter.
2. Check Collector health and local archive writes first: `podman-compose -f deploy/collector/compose.yaml ps`, inspect logs, and submit a local OTLP smoke record.
3. Check the management API health endpoint and its logs separately. A healthy HTTP endpoint does not replace checking database disk space, filesystem permissions, or backup status.
4. When the control plane returns, confirm the agent fetches and acknowledges its assigned version. Do not manually delete the last-known-good cache to force a sync.

## Rejected configuration or failed activation

1. The agent validates the candidate with the bundled Collector's `validate` command before activation.
2. On restart or health-check failure, the runtime restores the previous active config and restarts the previous pipeline. The cache is not advanced unless activation succeeds.
3. Inspect the agent/Collector logs for the failure, correct the policy, and publish a new version. Do not re-publish a different payload under the same version.
4. Verify health, OTLP ingestion, raw archive completeness, optimized output, and the applied version after recovery.

## SQLite backup and restore

The management API database defaults to `data/management.db`; the container deployment uses the persistent `management-data` volume.

### Create a consistent backup

Use SQLite's online backup API rather than copying the database file while WAL may be active:

```bash
scripts/management-backup.sh /path/to/management.db /path/to/private-backups
```

The script creates a timestamped SQLite backup, runs `PRAGMA integrity_check`, applies restrictive permissions (directory 0700, file 0600), and only publishes the completed file after validation. Copy completed backups to encrypted off-host storage using your organization-approved mechanism. Do not put secrets or backup artifacts in the repository.

### Restore a backup

1. Stop every management API instance and prevent automatic restart. Never restore while a writer is active.
2. Preserve the current database and its `-wal`/`-shm` sidecars for forensic recovery, if present.
3. Restore to a new, non-existing target path:

   ```bash
   scripts/management-restore.sh /path/to/management-YYYYMMDDTHHMMSSZ.db /path/to/management.db
   ```

4. Start one management API instance and verify health, tenant/agent/policy records, OIDC authorization, and agent configuration polling before reopening traffic.
5. Record the restore point and observed data loss, if any. Re-enable replicas only after the restored state is confirmed.

The restore script validates the source and staged copy and refuses to overwrite an existing database. It deliberately does not stop services or move existing databases automatically.

### Legacy JSON migration

Stop the service and move the old JSON file to the configured DB path. Startup preserves it as `.legacy-json.bak` and imports its state into SQLite. Keep that backup until the migrated service has been verified.

## Recovery drills

Run a restore drill before each production release and at least quarterly thereafter. Use an isolated copy of the database, not production. Record backup age, restore duration, integrity-check output, record counts, and whether a test agent can retrieve its assigned config. A successful integrity check proves SQLite structural integrity, not application-level completeness.

## Boundaries and production gates

SQLite WAL is intended for a local persistent filesystem and multiple processes on one host; it is not suitable for a multi-host control plane or NFS. Before horizontal SaaS scaling, migrate to PostgreSQL or an equivalent server database. Keep the management service on loopback/private networking behind TLS, store bootstrap and agent credentials in a secret manager, and review OIDC tenant/role claims with the identity provider. These scripts do not encrypt backups; encryption and off-host retention must be supplied by the operator.


## PostgreSQL encrypted backup and restore

For PostgreSQL deployments, use the encrypted helper rather than the SQLite scripts. Install PostgreSQL client tools and `age`; store the age **recipient** (public key) in deployment configuration and keep the corresponding identity (private key) in a separate secret manager or offline recovery location.

Create a validated custom-format dump and encrypt it before publication:

```bash
scripts/management-backup-postgres-encrypted.sh "$SIGNALFORGE_MANAGEMENT_DATABASE_URL" "age1..." /path/to/encrypted-off-host-staging
```

The script writes the plaintext dump only into a mode-0700 temporary directory, validates the dump, encrypts it with `age`, applies mode 0600, and atomically publishes only the encrypted artifact. Transfer that artifact to encrypted, access-controlled off-host storage using the organization's approved retention policy.

Restore into a **new empty PostgreSQL database**:

```bash
scripts/management-restore-postgres-encrypted.sh "$SIGNALFORGE_RESTORE_DATABASE_URL" /path/to/management-YYYYMMDDTHHMMSSZ.dump.age /secure/path/age-identity.txt
```

The restore helper decrypts to a private temporary directory, validates the archive, and refuses to restore if the target has public-schema objects. It does not create databases or stop services. Keep the target isolated, verify tenant/agent/policy records and agent polling, and only then plan a controlled cutover.

CI creates an ephemeral age identity and PostgreSQL restore database and exercises backup, encryption, restore, data verification, and the non-empty-target guard. This is automated evidence for the script path; operators must still run scheduled off-host drills, test access to the real private key, and record recovery time and recovery point. Never commit a private age identity, database URL, plaintext dump, or backup artifact.
