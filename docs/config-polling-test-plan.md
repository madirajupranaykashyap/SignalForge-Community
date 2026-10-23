# Config polling integration test plan

The Phase 19 unit tests cover authenticated fetch/acknowledgement, successful activation, rejected-config acknowledgement, no-assignment behavior, HTTPS enforcement for remote endpoints, request timeout defaults, and backoff jitter bounds.

The implemented integration adds tests for transactional storage serialization, legacy JSON import, validation rejection, and restart/health rollback. Before production use, additionally test against the actual management service and Collector distribution:

1. Verify tenant/agent credential isolation, expiry/revocation, and TLS certificate validation.
2. Test HTTP 401/403, 404, 429, 5xx, malformed JSON, oversized response, slow response, and cancellation.
3. Exercise invalid Collector component names and exporter/processor settings using the exact deployed distribution's validator.
4. Confirm rejected candidates never replace the last-known-good file or live pipeline.
5. Inject failures at activation, cache rename, acknowledgement, and process restart; verify durable/runtime configuration consistency.
6. Block the control plane and prove OTLP ingestion, local optimization, and archive writes continue while polling backs off.
7. Test credential rotation and revocation without logging bearer tokens.


## CI-covered release checks

- `go test -race ./...` and `go vet ./...` cover management, config cache/client, SQLite transaction handling, and the Collector runtime adapter.
- Collector image CI validates that the agent wrapper starts the bundled Collector and the health endpoint becomes ready.
- Runtime unit tests inject a candidate restart failure and assert the old active config is restored and the old process is restarted.

Target-environment tests remain necessary because CI cannot prove the deployed identity provider, TLS proxy, filesystem semantics, credentials, or actual production load.
