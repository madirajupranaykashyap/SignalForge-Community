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

## SQLite backup, migration, and restore

- The management API database is `data/management.db` by default; the container deployment stores it in the persistent `management-data` volume.
- For a live database, use SQLite's online backup API/CLI (for example, `sqlite3 management.db ".backup 'management.db.backup'"`). Do not copy only the main database file while the WAL is active.
- To migrate legacy JSON, stop the service and move the old JSON file to the configured DB path. Startup preserves it as `.legacy-json.bak` and imports its state into SQLite. Keep the backup until the migrated service has been verified.
- To restore, stop all writers, restore the database and any required filesystem permissions from a known-good backup, then start the service and verify tenant, agent, and policy records.
- Keep encrypted backups off-host and test restoration regularly.

## Boundaries

SQLite WAL is intended for a local persistent filesystem and multiple processes on one host; it is not suitable for a multi-host control plane or NFS. Before horizontal SaaS scaling, migrate to PostgreSQL or an equivalent server database. Keep the management service on loopback/private networking behind TLS, store bootstrap and agent credentials in a secret manager, and review OIDC tenant/role claims with the identity provider.
