# Phase 19: authenticated remote configuration polling

The `internal/configclient` package connects the enrolled agent credential and management API to the Phase 18 last-known-good cache. It is wired into the shipped `signalforge-agent` runtime, which supervises the bundled OpenTelemetry Collector process.

## Endpoint and transport

- Fetch: `GET /api/v1/agents/config`
- Acknowledge: `POST /api/v1/agents/config/ack`
- Both requests use the enrolled agent credential as a bearer token.
- HTTPS is required for remote endpoints. Plain HTTP is allowed only for loopback development endpoints.
- Requests have a 10-second default timeout, responses are capped at 4 MiB, and acknowledgements contain only a bounded diagnostic message.

## Poll and failure behavior

- The poller fetches an immutable versioned envelope, validates it through the exact bundled Collector binary, and delegates activation to the Phase 18 cache contract. The runtime stages the candidate, restarts the Collector, and waits for the health endpoint before the cache commits the new last-known-good version.
- It acknowledges a version as applied only after refresh succeeds; validation/activation errors produce a rejected acknowledgement.
- No assigned policy is a healthy no-op and does not create a false acknowledgement.
- Failures retry with bounded exponential backoff and jitter. The cache remains available to restore last-known-good configuration; the caller must keep local OTLP processing independent from this poll loop.
- Status includes last poll, last successful sync, active version, running state, and a bounded last error. It intentionally omits credentials.

## Digest compatibility

The management API now canonicalizes JSON policy payloads before computing the digest. The returned payload and digest therefore remain consistent after JSON encoding, even when an admin submitted insignificant whitespace.

## Remaining release gates

The adapter uses a process restart rather than an in-process hot reload. It validates before activation and restores the prior config if restart or health verification fails. Polling failure does not stop the current local pipeline. Production rollout still needs multi-host persistence (SQLite is single-host), provider-specific OIDC claim review, secure secret delivery, backup/restore drills, rate limiting, and failure injection on the target deployment.
