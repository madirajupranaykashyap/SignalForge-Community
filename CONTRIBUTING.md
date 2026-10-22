# Contributing to SignalForge

Thank you for your interest in contributing to SignalForge!

SignalForge is an open-source telemetry optimization platform focused on reducing observability costs while preserving important signals and supporting customer-controlled telemetry retention.

## Ways to contribute

You can contribute by:
- Reporting bugs and unexpected telemetry loss.
- Suggesting features and optimization policies.
- Improving documentation and examples.
- Fixing bugs and adding automated tests.
- Improving performance, reliability, and security.
- Testing integrations with OpenTelemetry-compatible backends.

## Before making changes

1. Check existing issues and pull requests to avoid duplicate work.
2. For significant changes, open an issue to discuss the proposed design first.
3. Keep changes focused and explain the problem they solve.
4. Never commit credentials, API keys, customer telemetry, or other sensitive information.

## Development workflow

1. Fork the repository or create a feature branch.
2. Make your changes in a dedicated branch.
3. Add or update tests for changed behavior.
4. Run formatting, tests, and applicable validation checks.
5. Update documentation when behavior or configuration changes.
6. Submit a pull request describing the change, its motivation, and how it was tested.

Suggested branch names:

- `feature/<short-description>`
- `fix/<short-description>`
- `docs/<short-description>`

## Code quality

- Follow standard Go conventions and run `gofmt`.
- Prefer small, focused functions and clear error handling.
- Avoid unnecessary dependencies.
- Add tests for normal behavior, edge cases, and failure scenarios.
- Never silently discard telemetry or conceal data loss.
- Document configuration options, defaults, and compatibility implications.

## Telemetry and reliability requirements

Changes to telemetry pipelines should consider:

- Preservation of error and important diagnostic signals.
- Behavior when exporters, archives, or downstream services are unavailable.
- Memory and CPU overhead under load.
- Retry behavior, buffering, and restart recovery.
- Privacy, redaction, retention, and access controls.

Optimization claims should include reproducible measurements where practical.

## Pull requests

A pull request should include:

- A clear summary and motivation.
- Tests or validation performed.
- Relevant documentation updates.
- Performance or compatibility impacts.
- Known limitations and follow-up work.

All automated checks must pass before a change is merged, unless a maintainer explicitly documents an exception.

## Security issues

Do not publish exploitable vulnerabilities or secrets in public issues. Report security concerns privately through the repository's configured security reporting channel.

## License

By submitting a contribution, you agree that your contribution will be distributed under the Apache License 2.0 for the applicable community-licensed project components, subject to the repository's contribution and licensing policies.

Please do not submit code that you do not have the right to contribute.

## Community standards

Be respectful, constructive, and open to technical feedback. Focus discussions on the code, design, and evidence rather than individuals.
