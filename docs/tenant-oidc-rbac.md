# Tenant OIDC authentication and RBAC (Phase 20)

Phase 20 adds an OIDC RS256 bearer-token verifier and tenant-scoped user endpoints to the management API. It is a security foundation, not a claim that the management service is production-ready.

## Configuration

Set all three variables to enable tenant-user endpoints:

- `SIGNALFORGE_OIDC_ISSUER`: exact issuer URL expected in the signed `iss` claim.
- `SIGNALFORGE_OIDC_AUDIENCE`: API audience expected in the signed `aud` claim.
- `SIGNALFORGE_OIDC_JWKS_URL`: HTTPS URL that returns the provider's JSON Web Key Set.

Optional claim-name overrides:

- `SIGNALFORGE_OIDC_TENANT_CLAIM`: defaults to `tenant_id`.
- `SIGNALFORGE_OIDC_ROLES_CLAIM`: defaults to `roles`.

If any required OIDC variable is set, all required values must be valid or startup fails. If OIDC is not configured, tenant endpoints return 503; they do not fall back to the platform bootstrap token. HTTP JWKS is allowed only for loopback development. The verifier accepts RS256 only, validates signature, issuer, audience, expiration and not-before, requires a subject and tenant claim, limits JWKS responses, uses bounded HTTP timeouts, refuses redirects, and caches keys briefly. Do not disable TLS validation in production.

The identity provider must issue a signed tenant claim and a roles array. SignalForge does not infer tenant membership from URL parameters or request bodies.

## Tenant routes

| Method | Route | Required role | Behavior |
| --- | --- | --- | --- |
| GET | `/api/v1/tenants/{tenantID}/agents` | `tenant_viewer`, `tenant_operator`, or `tenant_admin` | Lists only the authenticated tenant's agents; never returns credential hashes. |
| POST | `/api/v1/tenants/{tenantID}/policies` | `tenant_admin` | Creates an immutable policy version, assigns it to that tenant's active agents, and records the authenticated subject in the audit event. |

The signed tenant claim must exactly match `{tenantID}`; mismatch returns 404 to avoid disclosing cross-tenant resource existence. Authenticated users without the required role receive 403. A `platform_admin` role satisfies tenant role checks, but platform-wide administration still uses the separate bootstrap bearer routes.

Existing `/api/v1/admin/*` routes remain protected by `SIGNALFORGE_ADMIN_TOKEN`; that credential is intended for platform bootstrap/admin operations, not ordinary tenant users. Agent enrollment, heartbeat, config polling, and acknowledgements continue to use their individual agent credentials.

## Current limits and remaining release gates

- Persistence defaults to SQLite for single-host installs; Phase 23 adds PostgreSQL transactions with a row lock for multi-instance deployments. The state is still stored as one JSON snapshot rather than normalized tenant/agent/policy tables.
- OIDC user-to-tenant membership and role assignment are trusted from configured claims; provider-specific claim mapping and authorization policy still require deployment review.
- No tenant UI, refresh-token flow, or dynamic client registration is included. Phase 22 adds API-level per-source rate limiting and security headers; deployment-specific TLS, trusted proxy policy, and identity-provider claim mapping remain operator responsibilities.
- Policy payloads are digest-normalized but are not yet validated against a selected OpenTelemetry Collector distribution schema.
- Provider-specific tenant/role claim mapping must still be reviewed against the actual identity provider. PostgreSQL backup/restore helpers and automated outage-retention tests are added in subsequent phases, but real-environment restore/failover drills remain required.

Keep the management API on loopback or a private network behind TLS until those release gates are complete.
