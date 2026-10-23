# SignalForge

**Keep the signal. Cut the spend.**

SignalForge is a vendor-neutral telemetry optimization platform that helps engineering teams reduce observability costs while preserving important signals and retaining telemetry according to customer-defined policies.

## Architecture

- **Data plane:** Customer-hosted Go-based OpenTelemetry Collector distribution.
- **Control plane:** SaaS for configuration, worker management, analytics, and savings reporting.
- **Archive:** Customer-controlled object storage.
- **Vendor integrations:** OpenTelemetry-compatible and vendor-specific backends.

## Project status

Early development. The Collector accepts OTLP logs, traces, and metrics. The first policy-driven optimization baseline drops DEBUG logs and known health-check noise from the downstream log stream while preserving received logs in the raw archive. Traces and metrics currently go to debug output and are not durably archived. When configured with an enrolled agent credential, the Collector agent polls the management API, validates remote configs using the actual bundled Collector binary, and activates changes with restart/health-check rollback.

## Deployment

A Podman Compose package for the custom OpenTelemetry Collector is available in [deploy/collector](deploy/collector/README.md). It binds OTLP ports to loopback by default and persists received logs in a JSONL archive. A separate local management API package is documented in [deploy/management](deploy/management/README.md). Review both deployment guides before exposing endpoints to other hosts.

## Development

See [docs/architecture.md](docs/architecture.md) for the architecture and development plan, and [the operations recovery runbook](docs/operations-recovery.md) for outage, rollback, and database recovery procedures. The runbook documents validated SQLite backup/restore helper scripts.

## License

Community components are licensed under Apache License 2.0. Check each component's license and third-party notices before redistribution.

The initial optimization measurement baseline emits separate input and post-policy log-record count metrics. It does not yet claim byte or monetary savings.

The Collector exposes input and post-optimization log count metrics through a loopback-bound Prometheus endpoint by default; see the deployment guide.

## Control plane roadmap

- Phase 17 defines trust boundaries in [the control-plane foundation design](docs/control-plane-foundation.md). The single-process management API reference is documented in [docs/management-api.md](docs/management-api.md).
- Phase 18 adds a transport-neutral last-known-good configuration cache foundation in [docs/config-cache.md](docs/config-cache.md), including digest checks, schema-validator hooks, atomic cache replacement, and restart restore tests.
- Phase 19 adds an authenticated remote config polling client with bounded request sizes/timeouts, HTTPS enforcement for non-loopback endpoints, applied/rejected acknowledgements, and retry backoff with jitter. See [the polling client contract](docs/config-polling-client.md) and [test plan](docs/config-polling-test-plan.md).
- Phase 20 adds OIDC RS256 verification and tenant-scoped role-based API routes. See [Tenant OIDC and RBAC](docs/tenant-oidc-rbac.md).

The follow-up integration phase adds SQLite transactional state with a legacy JSON import/backup path, a Collector agent runtime adapter, distribution-specific validation through the shipped Collector binary, and rollback on failed restart/health checks. CI tests storage transactions, validation failures, restart rollback, and the Collector container smoke path. This is still not a multi-host SaaS control plane: SQLite is intended for a single-host/local durable volume, and production identity-provider claim mapping, PostgreSQL or equivalent distributed persistence, TLS deployment, rate limiting, and real-environment outage testing remain release gates.

## Phase 21: repeatable management database recovery

Adds operator-invoked SQLite backup and restore helpers that use the SQLite integrity check, restrictive file permissions, atomic publication of completed backups, and a restore flow that refuses to overwrite an existing database. See [the operations recovery runbook](docs/operations-recovery.md). This is a single-host recovery improvement, not a substitute for encrypted off-host backups, restore drills, or PostgreSQL before multi-host deployment.

## Phase 22: management API security hardening

Adds a bounded per-source token-bucket rate limit (default 120 requests per minute, configurable with `SIGNALFORGE_RATE_LIMIT_RPM`), HTTP 429 with `Retry-After`, browser security headers, and `Cache-Control: no-store` for API responses. Health probes are exempt from rate limiting. The API deliberately does not trust forwarded IP headers; deployments behind a reverse proxy must account for the proxy's shared source address. These controls complement but do not replace TLS, network restrictions, strong secrets, and provider-specific OIDC claim review.
