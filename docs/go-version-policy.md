# Go and Toolchain Version Policy

## Purpose

SignalForge uses Go for its telemetry data plane and backend services. This policy defines the Go versions used to build, test, and release the project.

## Supported version

- The `go` directive in `go.mod` defines the minimum Go version required by the module.
- The Go toolchain used in development and CI must satisfy that requirement.
- The selected OpenTelemetry Collector version and its dependencies may impose additional Go version requirements.
- The project must not claim support for a Go version that cannot build and test the pinned dependencies.

## Version selection

The project maintainers select the Go version based on:

1. Compatibility with the pinned OpenTelemetry Collector release.
2. Upstream Go support and security updates.
3. Dependency compatibility.
4. Reproducible builds across developer machines and CI.

The selected version must be recorded consistently in `go.mod`, CI workflows, and development documentation where applicable.

## Upgrades

The Go version should be reviewed when:

- The current Go release approaches the end of its support period.
- A security update requires an upgrade.
- An OpenTelemetry Collector upgrade changes the minimum requirement.
- A dependency requires a newer Go version.

Version upgrades should be made through a dedicated change that updates the relevant configuration and verifies the complete test suite.

## Compatibility

- Keep `go.mod` and `go.sum` committed.
- Pin production build and CI environments to an explicit Go version or an intentionally controlled version range.
- Avoid depending on undocumented behavior specific to one patch release.
- Test builds on the minimum supported version and the selected current version when practical.
- Document breaking changes caused by Go or dependency upgrades.

## Release builds

Release builds must use the Go version declared by the project's release configuration. CI should verify formatting, tests, and compilation before publishing artifacts.

## Policy maintenance

Maintainers may update this policy as Go and OpenTelemetry Collector requirements evolve. The repository configuration and automated checks take precedence over outdated examples in documentation.
