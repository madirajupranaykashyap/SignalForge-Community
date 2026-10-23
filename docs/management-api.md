# SignalForge management API reference

Run with `go run ./cmd/management`. The service defaults to `127.0.0.1:8081`.

## Configuration

- `SIGNALFORGE_ADMIN_TOKEN`: required; at least 32 characters.
- `SIGNALFORGE_MANAGEMENT_ADDR`: listen address, loopback by default.
- `SIGNALFORGE_MANAGEMENT_DB`: SQLite database path; defaults to `data/management.db`. The directory is created with restrictive permissions. Use a persistent local filesystem volume; do not place SQLite WAL databases on NFS or shared network filesystems.
- `SIGNALFORGE_MANAGEMENT_DATABASE_URL`: optional PostgreSQL URL; takes precedence over the SQLite path. Production deployments must use TLS with certificate verification, such as `sslmode=verify-full`.
- `SIGNALFORGE_OIDC_ISSUER`, `SIGNALFORGE_OIDC_AUDIENCE`, and `SIGNALFORGE_OIDC_JWKS_URL`: configure tenant-user authentication. If any is set, all must be valid or startup fails.
- `SIGNALFORGE_OIDC_TENANT_CLAIM`: optional tenant claim name; defaults to `tenant_id`.
- `SIGNALFORGE_OIDC_ROLES_CLAIM`: optional role claim name; defaults to `roles`.

## Platform admin endpoints

These endpoints use `Authorization: Bearer $SIGNALFORGE_ADMIN_TOKEN` and are reserved for platform bootstrap/admin operations:

- `POST /api/v1/admin/tenants`: `{"tenant_id":"acme"}`
- `POST /api/v1/admin/enrollment-tokens`: `{"tenant_id":"acme","ttl_seconds":900}` (60–86400 seconds)
- `POST /api/v1/admin/policies`: `{"tenant_id":"acme","schema_version":"1","config":{...}}`
- `POST /api/v1/admin/agents/{agentID}/revoke`

## Tenant-user endpoints

These endpoints use OIDC bearer access tokens validated against the configured issuer, audience, and JWKS:

- `GET /api/v1/tenants/{tenantID}/agents`: requires `tenant_viewer`, `tenant_operator`, or `tenant_admin`.
- `POST /api/v1/tenants/{tenantID}/policies`: requires `tenant_admin`; creates a version and assigns it to active agents in that tenant.

The signed tenant claim must match the tenant ID in the URL. A mismatch returns 404. Authenticated callers missing a required role receive 403. Tenant endpoints return 503 when OIDC is not configured and never fall back to the bootstrap token. See [Tenant OIDC and RBAC](tenant-oidc-rbac.md) for token validation and limitations.

## Agent endpoints

These endpoints use the one-time credential returned at enrollment:

- `POST /api/v1/agents/enroll`: consumes an enrollment token.
- `POST /api/v1/agents/heartbeat`
- `GET /api/v1/agents/config`
- `POST /api/v1/agents/config/ack`

## Maturity and release gates

State defaults to SQLite and each request is wrapped in a transaction. Set `SIGNALFORGE_MANAGEMENT_DATABASE_URL` to select PostgreSQL; the row lock around the state snapshot serializes mutations across replicas. PostgreSQL is the recommended backend for multi-host control planes. To migrate legacy JSON, stop the service, move the old JSON file to the configured DB path, and start the service; it will preserve a `.legacy-json.bak` copy and import the state. Back up a live DB using SQLite’s online backup mechanism, not by copying only the main file while WAL is active. The shipped Collector agent now validates config candidates using the bundled Collector binary and rolls back failed restart/health checks. Remote control-plane failure does not stop the existing Collector process; the poller backs off while the active local pipeline continues. OIDC tenant and role claims still require provider-specific mapping and review. Do not expose the service publicly or use it for real customer tenants until TLS termination, secret management, provider-specific OIDC claim review, encrypted off-host backups, restore drills, and target-environment outage tests are complete. PostgreSQL support is an implementation foundation, not a substitute for these operational gates.
