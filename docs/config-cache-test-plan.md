# Configuration cache test plan

The unit suite for `internal/configcache` covers:

- successful activation, durable write, restrictive file permissions, and restart restore;
- schema-validator rejection without replacing the previous cache;
- digest mismatch and stale-version rejection;
- activation failure without changing the in-memory or on-disk last-known-good version.

Before production enablement, add integration tests with the actual Collector distribution and control plane:

1. Reject unknown component names, unsupported schema versions, invalid pipelines, invalid exporter endpoints, and malformed processor settings.
2. Simulate control-plane timeout, 429, 5xx, DNS failure, and TLS errors while verifying local OTLP ingestion and archive writes continue.
3. Kill the process at each stage of config staging/activation/rename and confirm restart uses a valid last-known-good configuration.
4. Exercise rollback by assigning a previous immutable policy version and confirm it creates a new assignment/rollback event without rewriting policy history.
5. Verify no config or credential is logged, and ensure cache files remain 0600.
