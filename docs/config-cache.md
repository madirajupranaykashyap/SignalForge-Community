# Collector configuration cache (Phase 18)

The `internal/configcache` package is a transport-neutral building block for the customer-hosted agent. It does not route telemetry through the control plane and does not replace the Collector's local configuration at runtime by itself.

## Contract

- Configurations are immutable envelopes with a positive version, schema version, JSON payload, and SHA-256 digest over the exact payload bytes.
- The caller supplies a schema-aware validator. Syntactically valid JSON is not sufficient validation for an OpenTelemetry Collector configuration.
- The caller supplies an activator that must apply changes atomically: on error, the prior runtime config must remain active.
- The package stages the candidate in a same-directory temporary file with mode `0600` and fsync before activation. Only after successful activation does it atomically rename the staged file over the last-known-good cache.
- Invalid, stale, digest-mismatched, or failed-to-activate candidates do not replace the cached config. A restart can verify and restore the cached config.
- Equal version + equal digest is an idempotent no-op. Equal version with a different digest is rejected. Lower versions are rejected.

## Integration requirements

A future Collector adapter must implement the `Fetcher`, validate against the exact supported Collector schema/distribution, and make activation transactional (for example, validate a fully built replacement pipeline before swapping it into service). Polling should use bounded exponential backoff with jitter, deadlines, and a request timeout. The local OTLP and archive paths must not depend on a successful control-plane request. Do not interpret a failed control-plane fetch as a reason to clear or reset active configuration.

The package is a foundation, not proof of full end-to-end remote configuration: there is not yet an OIDC tenant identity layer, shared transactional control-plane database, live polling adapter, or Collector pipeline swap implementation. These remain release gates and are tracked in the control-plane foundation plan.
