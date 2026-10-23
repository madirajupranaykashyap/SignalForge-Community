# Config polling integration test plan

The Phase 19 unit tests cover authenticated fetch/acknowledgement, successful activation, rejected-config acknowledgement, no-assignment behavior, HTTPS enforcement for remote endpoints, request timeout defaults, and backoff jitter bounds.

Before production use, test against the actual management service and Collector distribution:

1. Verify tenant/agent credential isolation, expiry/revocation, and TLS certificate validation.
2. Test HTTP 401/403, 404, 429, 5xx, malformed JSON, oversized response, slow response, and cancellation.
3. Exercise invalid Collector component names and exporter/processor settings using the exact deployed distribution's validator.
4. Confirm rejected candidates never replace the last-known-good file or live pipeline.
5. Inject failures at activation, cache rename, acknowledgement, and process restart; verify durable/runtime configuration consistency.
6. Block the control plane and prove OTLP ingestion, local optimization, and archive writes continue while polling backs off.
7. Test credential rotation and revocation without logging bearer tokens.
