# SignalForge management API (Phase 17B)

This reference service is separate from the benchmark API and starts with
`go run ./cmd/management`. It defaults to `127.0.0.1:8081`; do not bind it
to a public interface without TLS termination, rate limiting, secret rotation,
and an external identity provider.

## Configuration

- `SIGNALFORGE_ADMIN_TOKEN`: required, at least 32 characters. Startup fails closed if missing/short.
- `SIGNALFORGE_MANAGEMENT_ADDR`: listen address; defaults to loopback.
- `SIGNALFORGE_MANAGEMENT_STATE`: state file; defaults to `data/management-state.json`. The file and containing directory are created with restrictive permissions and writes use temp-file + sync + rename.

## Endpoints

Admin endpoints require `Authorization: Bearer $SIGNALFORGE_ADMIN_TOKEN`:

- `POST /api/v1/admin/tenants` — `{"tenant_id":"acme"}`
- `POST /api/v1/admin/enrollment-tokens` — `{"tenant_id":"acme","ttl_seconds":900}`; returns a random token once, TTL 60–86400 seconds.
- `POST /api/v1/admin/policies` — `{"tenant_id":"acme","schema_version":"1","config":{...}}`; creates immutable version and assigns it to active tenant agents.
- `POST /api/v1/admin/agents/{agentID}/revoke` — revokes agent credential.

Agent endpoints use the one-time credential returned by enrollment:

- `POST /api/v1/agents/enroll` — `{"token":"..."}`; token is consumed once.
- `POST /api/v1/agents/heartbeat` — `{"version":"...","applied_version":0}`
- `GET /api/v1/agents/config`
- `POST /api/v1/agents/config/ack` — `{"version":1,"applied":true,"message":""}`

## Explicit maturity and security limits

This is a **single-process reference implementation**, not yet a production SaaS
identity system. The admin bearer token is a platform-level bootstrap credential,
not tenant-admin authentication; tenant-scoped user roles and external OIDC
integration are not implemented. JSON state is not safe for multiple replicas
or concurrent independent processes; migrate to a transactional database with
constraints before horizontal scaling. Config payloads are checked for valid
JSON and versioned/digested, but Collector-specific schema validation and
last-known-good activation belong in the agent and are not claimed here.
Deploy behind TLS, restrict admin access, back up state, and rotate the admin
credential before any real customer use.
