# SignalForge management API reference

Run with `go run ./cmd/management`. The service defaults to `127.0.0.1:8081`.

## Configuration
- `SIGNALFORGE_ADMIN_TOKEN`: required; at least 32 characters.
- `SIGNALFORGE_MANAGEMENT_ADDR`: listen address, loopback by default.
- `SIGNALFORGE_MANAGEMENT_STATE`: state file; defaults to `data/management-state.json`.

## Endpoints
Admin endpoints use `Authorization: Bearer $SIGNALFORGE_ADMIN_TOKEN`:
- `POST /api/v1/admin/tenants`: `{"tenant_id":"acme"}`
- `POST /api/v1/admin/enrollment-tokens`: `{"tenant_id":"acme","ttl_seconds":900}` (60–86400 seconds)
- `POST /api/v1/admin/policies`: `{"tenant_id":"acme","schema_version":"1","config":{...}}`
- `POST /api/v1/admin/agents/{agentID}/revoke`

Agent endpoints use the one-time credential returned at enrollment:
- `POST /api/v1/agents/enroll`: `{"token":"..."}`
- `POST /api/v1/agents/heartbeat`
- `GET /api/v1/agents/config`
- `POST /api/v1/agents/config/ack`

## Maturity and release gates
This is a single-process reference implementation, not a production SaaS identity system. The admin bearer is a platform bootstrap credential, not tenant-user OIDC/RBAC. JSON persistence is not suitable for multiple replicas or independent processes. Config payloads are versioned and digested but not validated against Collector schemas, and the Collector-side last-known-good cache/rollback client is not included. Use TLS and network restrictions before any non-loopback deployment. Do not expose this service publicly or use it for real customer tenants until OIDC/RBAC, transactional persistence, schema validation, backup/restore, and outage/rollback tests are implemented.
