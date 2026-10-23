# SignalForge Control Plane Foundation (Phase 17)

## Status

Design contract for the next implementation milestone. Enrollment and remote configuration endpoints are not implemented by this document.

## Goals

- Enroll customer-hosted Collectors without routing telemetry through the SaaS.
- Isolate tenant identities, agents, policies, and audit records.
- Deliver immutable, validated, versioned configuration.
- Keep the local Collector operational during control-plane outages.
- Support revocation, health reporting, and rollback.

## Trust boundaries

A tenant administrator manages enrollment tokens, agents, and policies only within their tenant. A Collector agent can heartbeat as itself, fetch its assigned configuration, and acknowledge application status. It cannot list other agents or author policies. Platform administration must be separately authorized.

Authorization derives tenant membership from the authenticated principal or agent credential, never from an untrusted tenant ID in a request body. Do not route customer telemetry through the control plane.

## Initial API contract

| Method | Endpoint | Purpose |
| --- | --- | --- |
| POST | /api/v1/agents/enroll | Consume a scoped enrollment token and issue an agent identity |
| POST | /api/v1/agents/heartbeat | Report agent version, health, and last-applied config version |
| GET | /api/v1/agents/config | Fetch config authorized for the authenticated agent |
| POST | /api/v1/agents/config/ack | Acknowledge applied/rejected config and validation details |
| GET | /api/v1/tenants/{tenantID}/agents | List agents in the authenticated tenant |
| POST | /api/v1/tenants/{tenantID}/policies | Create an immutable policy version |
| POST | /api/v1/tenants/{tenantID}/policies/{version}/validate | Validate without applying |
| POST | /api/v1/tenants/{tenantID}/agents/{agentID}/rollback | Assign a previous known-good config |

These are proposed routes, not implemented endpoints.

## Enrollment credential lifecycle

1. An authenticated tenant administrator creates a token with expiry and limited scope.
2. Generate tokens with cryptographically secure randomness and persist only their hashes.
3. Exchange the token over TLS; consume it and create the agent in one transaction to prevent replay.
4. Return a unique random agent credential once; persist only its hash.
5. Support credential rotation and revocation. Logs may contain credential identifiers, never credential values.

Do not ship a global static enrollment secret or an unauthenticated endpoint that creates tenant identities.

## Configuration lifecycle

Each policy/config version is immutable and includes schema version, policy version, creation actor/time, validation status, and digest. The agent validates downloaded config locally before activation, writes it atomically, and reports the result. Rejected config never replaces the last-known-good version. Rollback reassigns a previous immutable version instead of rewriting history.

The Collector polls with bounded exponential backoff and jitter. A control-plane outage must not block OTLP ingestion, local optimization, or customer-controlled archival. Cache last-known-good config and expose last successful sync time and last applied version as health metadata.

## Minimum persistent model

- Tenants and tenant status.
- External user identity subjects and tenant roles.
- Enrollment token hashes, scope, expiry, and consumed time.
- Agents, tenant association, credential hash, status, version, and last-seen time.
- Policies and immutable policy versions with schema and digest.
- Agent configuration assignments and acknowledgements.
- Audit events containing actor, action, resource, timestamp, and request ID.

Use a transactional database with foreign keys and uniqueness constraints. Multi-instance deployment requires shared durable storage and concurrency-safe token consumption.

## Error behavior

- 400: malformed request or schema validation failure.
- 401: missing or invalid credentials.
- 403: authenticated principal lacks permission.
- 404: resource absent within authorized tenant scope; do not disclose cross-tenant existence.
- 409: token already consumed or version conflict.
- 410: token expired or revoked.
- 422: syntactically valid but invalid policy/config.
- 429: rate limit exceeded.
- 5xx: transient failure; agents retry with backoff.

## Security and release gates

- Cross-tenant isolation tests for every resource operation.
- Single-use/expiry tests for enrollment tokens, including concurrent requests.
- Credential redaction and revocation tests.
- Database restart durability, migrations, backup and restore tests.
- Invalid-config rollback tests.
- Contract tests for polling, acknowledgement, and version compatibility.
- Outage test proving local processing continues with cached config.
- Threat model and secrets-management runbook.

## Follow-on implementation order

1. Add persistent tenant/agent/policy repositories and migrations.
2. Implement authentication, roles, enrollment-token issuance/consumption, and revocation.
3. Implement authenticated heartbeat, config fetch, and acknowledgements.
4. Implement policy validation, immutable versions, assignments, audit records, and rollback.
5. Add a Collector-side config client with cache, validation, and outage tests.
6. Add integration tests and a minimal tenant/agent management interface.
