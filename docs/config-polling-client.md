# Phase 19: authenticated remote configuration polling

The `internal/configclient` package connects the agent credential and management API to the Phase 18 last-known-good cache. It is a reusable client/poller, not yet a live OpenTelemetry Collector pipeline adapter.

## Endpoint and transport

- Fetch: `GET /api/v1/agents/config`
- Acknowledge: `POST /api/v1/agents/config/ack`
- Both requests use the enrolled agent credential as a bearer token.
- HTTPS is required for remote endpoints. Plain HTTP is allowed only for loopback development endpoints.
- Requests have a 10-second default timeout, responses are capped at 4 MiB, and acknowledgements contain only a bounded diagnostic message.

## Poll and failure behavior

- The poller fetches an immutable versioned envelope, delegates schema validation, and delegates activation to the Phase 18 cache contract.
- It acknowledges a version as applied only after refresh succeeds; validation/activation errors produce a rejected acknowledgement.
- No assigned policy is a healthy no-op and does not create a false acknowledgement.
- Failures retry with bounded exponential backoff and jitter. The cache remains available to restore last-known-good configuration; the caller must keep local OTLP processing independent from this poll loop.
- Status includes last poll, last successful sync, active version, running state, and a bounded last error. It intentionally omits credentials.

## Digest compatibility

The management API now canonicalizes JSON policy payloads before computing the digest. The returned payload and digest therefore remain consistent after JSON encoding, even when an admin submitted insignificant whitespace.

## Remaining release gates

This package still requires a deployment-specific Collector schema validator and a transactional pipeline activator. No production runtime currently wires this poller into an OpenTelemetry Collector pipeline, exposes its status endpoint, or proves end-to-end local processing during control-plane outages. OIDC tenant-user authorization and shared transactional control-plane persistence also remain incomplete.
