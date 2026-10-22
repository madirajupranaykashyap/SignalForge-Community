# Go and Toolchain Version Policy

## Purpose

SignalForge uses Go for its telemetry data plane and backend services. This policy defines the required development toolchain and version-upgrade process.

## Standard Toolchain

- **Preferred Go toolchain:** `1.27.3`
- Developers should use Go `1.27.3` for local development and testing.
- CI and release builds should use the same Go version unless an approved compatibility test requires another version.
- The selected OpenTelemetry Collector release and its dependencies must be compatible with the chosen toolchain.

## Go Module Version

The `go` directive in each `go.mod` file declares the minimum Go version required by that module.

The module's Go version must be selected after checking the requirements of the pinned OpenTelemetry Collector release and its dependencies. Do not assume that the module's minimum version and the exact development toolchain must be identical.

## Dependency Management

- Commit `go.mod` and `go.sum`.
- Pin compatible OpenTelemetry Collector component versions.
- Review dependency upgrades for compatibility, security, and license implications.
- Avoid unreviewed dependency upgrades in release branches.

## Upgrades and Security

- Review Go security releases promptly.
- Update the standard toolchain when an approved security or maintenance release is available.
- Test the Collector distribution and SignalForge components before adopting a new toolchain.
- Update CI, build images, documentation, and version declarations together.

## Validation

Before merging a toolchain or dependency upgrade, run formatting, unit tests, and compilation checks. Validate the custom Collector build and telemetry pipelines where applicable.

## Exceptions

Any deviation from the standard toolchain must be documented in the relevant pull request, including the reason and compatibility impact.
