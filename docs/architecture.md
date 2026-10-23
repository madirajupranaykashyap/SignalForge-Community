
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
