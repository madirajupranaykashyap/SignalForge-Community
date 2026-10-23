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

The control plane is a management plane, not a telemetry proxy. Collector instances must continue processing locally when the control plane is unavailable. The trust-boundary and API contract are documented in [Control Plane Foundation](control-plane-foundation.md). The management API initially uses a platform bootstrap bearer and local JSON state, not shared transactional storage.

## Phase 18: last-known-good config cache foundation

The `internal/configcache` package verifies immutable config envelopes using SHA-256, delegates schema validation and atomic runtime activation to explicit callbacks, stages cache files with mode 0600, and restores the last-known-good envelope on restart. Invalid, stale, digest-mismatched, or failed-to-activate candidates do not replace the cache.

## Phase 19: authenticated remote config polling

The `internal/configclient` package polls the management API using an agent bearer credential, validates the versioned envelope via the Phase 18 cache, and acknowledges applied or rejected versions. Remote endpoints require HTTPS; loopback HTTP is allowed only for development. The poller uses bounded response sizes, request timeouts, exponential backoff, and jitter. A missing assignment is a healthy no-op.

## Phase 20: tenant OIDC authentication and RBAC

The `internal/oidcauth` package validates RS256 access tokens against a configured issuer, audience, and JWKS endpoint. The management API exposes tenant-scoped agent listing and policy creation routes, derives tenant identity from the signed token claim, enforces roles, redacts credential hashes, and records the authenticated subject for tenant policy audit events. Platform bootstrap routes remain separate. See [Tenant OIDC and RBAC](tenant-oidc-rbac.md).

Phase 20 does not make the management API production-ready: tenant and role claims still require identity-provider mapping review, state is still stored in a local JSON file, and the polling client is not wired to a live Collector pipeline. Shared transactional persistence, distribution-specific Collector schema validation, transactional pipeline activation, backup/restore, and end-to-end outage/rollback tests remain release gates.
