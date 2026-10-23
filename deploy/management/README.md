# SignalForge management API deployment

This is a single-host reference deployment for the management API. It binds to loopback by default, persists state in a local SQLite database volume, and requires a platform bootstrap token.

## Start

```bash
cd deploy/management
cp .env.example .env
# Put a strong SIGNALFORGE_ADMIN_TOKEN in your secret manager or local untracked .env.
podman-compose -f compose.yaml up -d --build
podman-compose -f compose.yaml ps
curl --fail http://127.0.0.1:8081/healthz
```

Generate an admin token with `openssl rand -hex 32`. Never commit the populated `.env` file or share the token. The endpoint is bound to `127.0.0.1` unless you explicitly override `MANAGEMENT_BIND`; changing the bind address does not add TLS. Put remote traffic behind a TLS-terminating reverse proxy and firewall.

## OIDC configuration

To enable tenant-user routes, configure `SIGNALFORGE_OIDC_ISSUER`, `SIGNALFORGE_OIDC_AUDIENCE`, and `SIGNALFORGE_OIDC_JWKS_URL` together. Use the identity provider's HTTPS issuer and HTTPS JWKS endpoint. Claim names default to `tenant_id` and `roles`. The provider must securely assign these claims; test tenant isolation before use.

## Database backup and migration

The named `management-data` volume stores `management.db`. Stop the service before doing a file-level copy, or use SQLite's online backup command against the live database. To migrate a legacy JSON state file, stop the service, place the JSON file at the configured database path, and restart; the application preserves it as `.legacy-json.bak` and imports the contents. Keep encrypted, off-host backups and rehearse restore.

SQLite WAL is suitable for a local persistent volume and concurrent processes on one host. It is **not** a multi-host SaaS database and must not be placed on NFS. Use PostgreSQL or an equivalent server database before horizontally scaling the control plane.

See [management API reference](../../docs/management-api.md) and [tenant OIDC/RBAC notes](../../docs/tenant-oidc-rbac.md).
