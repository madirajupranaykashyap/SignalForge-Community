
# SignalForge Architecture

## Design principles

1. Process raw telemetry inside the customer's environment.
2. Keep archive and vendor-output paths logically independent.
3. Preserve important error and latency signals through explicit policies.
4. Measure input, output, archive completeness, and processing overhead.
5. Keep the data plane operational when the SaaS control plane is unavailable.
6. Version configurations and support validation and rollback.

## Initial data flow

Applications -> OTLP receiver -> processing pipeline

The pipeline branches into:

- Archive: accepted telemetry subject to privacy and retention policies.
- Vendor: optimized telemetry sent to the configured observability backend.
- Analytics: aggregate measurements for savings reporting.

## Phase 1

Prove that the Collector receives telemetry, applies a reduction policy, delivers the expected output, and provides measurable evidence of the result.

Do not introduce the SaaS control plane until the data-plane behavior is demonstrated.


## Phase 5: optimization effectiveness measurements

Separate count connectors emit `signalforge.input.log.records` for the archive
branch and `signalforge.optimized.log.records` for the post-policy branch.
These counters support record-reduction calculations without treating debug
log text as a metric. Byte savings, processing overhead, and cost savings are
not yet measured. The counters currently flow through the debug exporter and
are not exposed through a dedicated Prometheus endpoint or remote backend.


## Phase 6: metrics endpoint

A Prometheus exporter exposes the optimization count connector metrics on
container port 9464. The Compose package binds the host port to loopback by
default. Monitoring systems can scrape the endpoint locally; remote exposure
requires explicit network controls. The metrics describe record counts only,
not bytes or monetary savings.


## Phase 17: control-plane foundation

The control plane is a management plane, not a telemetry proxy. Collector instances must continue processing locally when the control plane is unavailable. Phase 17 defines enrollment, tenant isolation, versioned configuration, health reporting, and auditable policy changes before enabling remote configuration delivery.

The API contract and trust boundaries are documented in [Control Plane Foundation](control-plane-foundation.md). The reference implementation in [Management API](management-api.md) provides single-process enrollment and policy lifecycle endpoints. It uses a bootstrap admin bearer token and local JSON state, so it is not a production multi-tenant SaaS identity system. Tenant-admin OIDC/RBAC, transactional shared storage, Collector-side config validation/cache, and last-known-good activation remain mandatory release gates.
