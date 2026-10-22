# SignalForge

**Keep the signal. Cut the spend.**

SignalForge is a vendor-neutral telemetry optimization platform that helps engineering teams reduce observability costs while preserving important signals and retaining telemetry according to customer-defined policies.

## Architecture

- **Data plane:** Customer-hosted Go-based OpenTelemetry Collector distribution.
- **Control plane:** SaaS for configuration, worker management, analytics, and savings reporting.
- **Archive:** Customer-controlled object storage.
- **Vendor integrations:** OpenTelemetry-compatible and vendor-specific backends.

## Project status

Early development. The initial milestone is to demonstrate telemetry reduction, archive completeness, and measurable pipeline overhead.

## Development

See `docs/architecture.md` for the architecture and development plan.

## License

Community components are licensed under Apache License 2.0. Check each component's license and third-party notices before redistribution.

