# SignalForge

**Keep the signal. Cut the spend.**

SignalForge is a vendor-neutral telemetry optimization platform that helps engineering teams reduce observability costs while preserving important signals and retaining telemetry according to customer-defined policies.

## Architecture

- **Data plane:** Customer-hosted Go-based OpenTelemetry Collector distribution.
- **Control plane:** SaaS for configuration, worker management, analytics, and savings reporting.
- **Archive:** Customer-controlled object storage.
- **Vendor integrations:** OpenTelemetry-compatible and vendor-specific backends.

## Project status

Early development. The Collector accepts OTLP logs, traces, and metrics. The first
policy-driven optimization baseline drops DEBUG logs and known health-check noise
from the downstream log stream while preserving all received logs in the raw archive.
Traces and metrics currently go to debug output and are not durably archived.

## Deployment

A Podman Compose package for the custom OpenTelemetry Collector is available in [deploy/collector](deploy/collector/README.md). It binds OTLP ports to loopback by default and persists received logs in a JSONL archive. Review the deployment guide before exposing OTLP endpoints to other hosts.

## Development

See `docs/architecture.md` for the architecture and development plan.

## License

Community components are licensed under Apache License 2.0. Check each component's license and third-party notices before redistribution.

The initial optimization measurement baseline emits separate input and
post-policy log-record count metrics. It does not yet claim byte or monetary
savings.

The Collector exposes input and post-optimization log count metrics through a
loopback-bound Prometheus endpoint by default; see the deployment guide.

## Control plane roadmap

Phase 17 defines trust boundaries in [the control-plane foundation design](docs/control-plane-foundation.md). A single-process management API reference is documented in [docs/management-api.md](docs/management-api.md). It demonstrates enrollment tokens, agent credentials, heartbeats, policy versioning, acknowledgements, and revocation; it is not production-ready until tenant-user OIDC/RBAC and transactional persistence are implemented.

Phase 18 adds a transport-neutral last-known-good configuration cache foundation in [docs/config-cache.md](docs/config-cache.md), including digest checks, schema-validator hooks, atomic cache replacement, and restart restore tests.

Phase 19 adds an authenticated remote config polling client with bounded request sizes/timeouts, HTTPS enforcement for non-loopback endpoints, applied/rejected acknowledgements, and retry backoff with jitter. See [the polling client contract](docs/config-polling-client.md) and [test plan](docs/config-polling-test-plan.md). This is not yet wired to a live Collector pipeline: a real Collector schema validator and transactional pipeline swap are still required. OIDC tenant-user authorization and shared transactional storage also remain release gates.
