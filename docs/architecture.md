# SignalForge Architecture

## Design principles

1. Process raw telemetry inside the customer's environment.
2. Keep archive and vendor-output paths logically independent.
3. Preserve important error and latency signals through explicit policies.
4. Measure input, output, archive completeness, and processing overhead.
5. Keep the data plane operational when the control plane is unavailable.
6. Version configurations and support validation and rollback.

## Initial data flow

Applications -> OTLP receiver -> processing pipeline

The pipeline branches into:

- Archive: accepted telemetry subject to privacy and retention policies.
- Vendor: optimized telemetry sent to the configured observability backend.
- Analytics: aggregate measurements for savings reporting.

## Phase 1

Prove that the Collector receives telemetry, applies a reduction policy, delivers the expected output, and provides measurable evidence of the result. Do not introduce the SaaS control plane until the data-plane behavior is demonstrated.

## Phase 5: optimization effectiveness measurements

Separate count connectors emit `signalforge.input.log.records` for the archive branch and `signalforge.optimized.log.records` for the post-policy branch. These counters support record-reduction calculations without treating debug log text as a metric. Byte savings, processing overhead, and cost savings are not yet measured.

## Phase 6: metrics endpoint

A Prometheus exporter exposes the optimization count connector metrics on container port 9464. The Compose package binds the host port to loopback by default. Monitoring systems can scrape the endpoint locally; remote exposure requires explicit network controls. The metrics describe record counts only, not bytes or monetary savings.

## Phase 17: control-plane foundation

The control plane is a management plane, not a telemetry proxy. Collector instances must continue processing locally when the control plane is unavailable. The trust-boundary and API contract are documented in [Control Plane Foundation](control-plane-foundation.md). The management API uses a platform bootstrap bearer and a durable SQLite state store. Each request reloads and updates the snapshot inside a write-reserving transaction, so concurrent writers sharing the same local database cannot overwrite stale state. Legacy JSON is preserved as a backup and imported during migration. SQLite WAL is a single-host/local-filesystem choice, not a multi-host SaaS database.

## Phase 18: last-known-good config cache foundation

The `internal/configcache` package verifies immutable config envelopes using SHA-256, delegates schema validation and atomic runtime activation to explicit callbacks, stages cache files with mode 0600, and restores the last-known-good envelope on restart. Invalid, stale, digest-mismatched, or failed-to-activate candidates do not replace the cache.

## Phase 19: authenticated remote config polling

The `internal/configclient` package polls the management API using an agent bearer credential, validates the versioned envelope via the Phase 18 cache, and acknowledges applied or rejected versions. Remote endpoints require HTTPS; loopback HTTP is allowed only for development. The poller uses bounded response sizes, request timeouts, exponential backoff, and jitter. A missing assignment is a healthy no-op. The shipped `signalforge-agent` now wires this poller into the Collector process.

## Phase 20: tenant OIDC authentication and RBAC

The `internal/oidcauth` package validates RS256 access tokens against a configured issuer, audience, and JWKS endpoint. The management API exposes tenant-scoped agent listing and policy creation routes, derives tenant identity from the signed token claim, enforces roles, redacts credential hashes, and records the authenticated subject for tenant policy audit events. Platform bootstrap routes remain separate. See [Tenant OIDC and RBAC](tenant-oidc-rbac.md).

Phase 20 alone did not make the management API production-ready. The follow-up integration now adds SQLite transactions, actual bundled-Collector validation, supervised process restart/health checks, rollback, and outage/restart tests. Remaining release gates include multi-host PostgreSQL or equivalent persistence, identity-provider claim mapping review, TLS and secret management, rate limiting, and backup/restore drills in the target environment.


## Follow-up integration: transactional storage, runtime activation, and recovery

- `internal/managementstore` stores the management snapshot in SQLite. Each HTTP request loads, mutates, and saves state within one IMMEDIATE transaction; SQLite serializes writers across processes on the same local filesystem. A legacy JSON file is backed up and imported on first startup when it occupies the configured DB path.
- `cmd/collector-agent` supervises the custom Collector binary. With an enrolled credential and `SIGNALFORGE_CONFIG_ENDPOINT`, it polls for policies; without remote-config settings it runs the baked-in local config and continues local processing.
- `internal/collectorhost` validates candidates through the exact bundled Collector executable, stages them with restrictive permissions, restarts the process, and waits for its health endpoint. A failed restart or health check restores the prior config and restarts the previous pipeline. The config cache is updated only after activation succeeds.
- CI covers storage transaction serialization, legacy import, validation rejection, restart failure rollback, the management API, and the Collector image smoke tests.

The control plane and Collector agent are still early-stage. SQLite WAL requires local persistent storage and is not a multi-host database; use PostgreSQL or another server database before multi-replica SaaS deployment. Tenant/role claims need provider-specific review, and production rollout still requires backup/restore drills, secrets management, rate limiting, and failure injection in the target environment.
