# SignalForge management API reference

Run with `go run ./cmd/management`. The service defaults to `127.0.0.1:8081`.

## Configuration

- `SIGNALFORGE_ADMIN_TOKEN`: required; at least 32 characters.
- `SIGNALFORGE_MANAGEMENT_ADDR`: listen address, loopback by default.
- `SIGNALFORGE_MANAGEMENT_STATE`: state file; defaults to `data/management-state.json`.
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

This remains a single-process reference implementation, not a production SaaS identity system. JSON persistence is not suitable for multiple replicas or independent processes. OIDC role and tenant claims require provider-specific mapping and review. Config payloads are versioned and digested after canonical JSON normalization, but are not validated against a selected Collector distribution schema. The Phase 18 cache and Phase 19 polling client are reusable packages and are not yet wired into an actual Collector runtime. Use TLS and network restrictions before any non-loopback deployment. Do not expose this service publicly or use it for real customer tenants until shared transactional persistence, a deployment-specific Collector schema validator, transactional pipeline activation, backup/restore, and end-to-end outage/rollback tests are complete.
