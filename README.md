# SignalForge

**Keep the signal. Cut the spend.**

SignalForge is a vendor-neutral telemetry optimization platform that helps engineering teams reduce observability costs while preserving important signals and retaining telemetry according to customer-defined policies.

## Architecture

- **Data plane:** Customer-hosted Go-based OpenTelemetry Collector distribution.
- **Control plane:** SaaS for configuration, worker management, analytics, and savings reporting.
- **Archive:** Customer-controlled object storage.
- **Vendor integrations:** OpenTelemetry-compatible and vendor-specific backends.

## Project status

Early development. The Collector accepts OTLP logs, traces, and metrics. The first policy-driven optimization baseline drops DEBUG logs and known health-check noise from the downstream log stream while preserving all received logs in the raw archive. Traces and metrics currently go to debug output and are not durably archived.

## Deployment

A Podman Compose package for the custom OpenTelemetry Collector is available in [deploy/collector](deploy/collector/README.md). It binds OTLP ports to loopback by default and persists received logs in a JSONL archive. Review the deployment guide before exposing OTLP endpoints to other hosts.

## Development

See [docs/architecture.md](docs/architecture.md) for the architecture and development plan.

## License

Community components are licensed under Apache License 2.0. Check each component's license and third-party notices before redistribution.

The initial optimization measurement baseline emits separate input and post-policy log-record count metrics. It does not yet claim byte or monetary savings.

The Collector exposes input and post-optimization log count metrics through a loopback-bound Prometheus endpoint by default; see the deployment guide.

## Control plane roadmap

- Phase 17 defines trust boundaries in [the control-plane foundation design](docs/control-plane-foundation.md). The single-process management API reference is documented in [docs/management-api.md](docs/management-api.md).
- Phase 18 adds a transport-neutral last-known-good configuration cache foundation in [docs/config-cache.md](docs/config-cache.md), including digest checks, schema-validator hooks, atomic cache replacement, and restart restore tests.
- Phase 19 adds an authenticated remote config polling client with bounded request sizes/timeouts, HTTPS enforcement for non-loopback endpoints, applied/rejected acknowledgements, and retry backoff with jitter. See [the polling client contract](docs/config-polling-client.md) and [test plan](docs/config-polling-test-plan.md).
- Phase 20 adds OIDC RS256 verification and tenant-scoped role-based API routes. See [Tenant OIDC and RBAC](docs/tenant-oidc-rbac.md).

These phases are foundations, not production readiness. The management API still uses a local JSON file, the config client is not wired into a live Collector pipeline, and a distribution-specific Collector schema validator plus transactional pipeline activation remain release gates. Do not expose the management service publicly or use it for real customer tenants until transactional shared persistence, identity/role mapping review, backup/restore, and end-to-end outage/rollback tests are complete.
