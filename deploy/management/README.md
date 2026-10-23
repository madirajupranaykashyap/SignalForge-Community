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

## Request limits and response protections

The API applies a per-source token-bucket limit to requests other than `/healthz`. The default is 120 requests per minute per source address. Override with `SIGNALFORGE_RATE_LIMIT_RPM` (integer from 1 through 10000) based on expected agent polling and tenant traffic. Rate-limited requests return HTTP 429 and `Retry-After`. The health endpoint is exempt to keep liveness checks stable during API bursts.

Source identity uses the TCP peer address; the API intentionally does not trust `X-Forwarded-For` or similar caller-supplied headers. Behind a reverse proxy, requests may share the proxy address and therefore a rate bucket. If per-client limiting is needed behind a proxy, configure trusted client identity at the proxy/firewall or implement an explicit trusted-proxy policy rather than blindly trusting forwarding headers.

Responses include basic browser security headers. API responses use `Cache-Control: no-store`. These controls are defense-in-depth, not a replacement for authentication, TLS, network policy, or identity-provider review. The application does not enable HSTS itself because TLS is expected to terminate at the deployment edge.

## OIDC configuration

To enable tenant-user routes, configure `SIGNALFORGE_OIDC_ISSUER`, `SIGNALFORGE_OIDC_AUDIENCE`, and `SIGNALFORGE_OIDC_JWKS_URL` together. Use the identity provider's HTTPS issuer and HTTPS JWKS endpoint. Claim names default to `tenant_id` and `roles`. The provider must securely assign these claims; test tenant isolation before use.

## Database backup and migration

The named `management-data` volume stores `management.db`. Stop the service before doing a file-level copy, or use SQLite's online backup command against the live database. To migrate a legacy JSON state file, stop the service, place the JSON file at the configured database path, and restart; the application preserves it as `.legacy-json.bak` and imports the contents. Keep encrypted, off-host backups and rehearse restore.

SQLite WAL is suitable for a local persistent volume and concurrent processes on one host. It is **not** a multi-host SaaS database and must not be placed on NFS. Use PostgreSQL or an equivalent server database before horizontally scaling the control plane.

See [management API reference](../../docs/management-api.md) and [tenant OIDC/RBAC notes](../../docs/tenant-oidc-rbac.md).


## Secret files and TLS reverse proxy

The management process accepts `SIGNALFORGE_ADMIN_TOKEN_FILE` and `SIGNALFORGE_MANAGEMENT_DATABASE_URL_FILE`. Each points to a readable file **inside the container**; the corresponding direct environment variable must be empty. The file content is read at startup, with trailing line endings removed. Mount secret files read-only through your deployment's secret mechanism or a Compose override. Do not bake secrets into the image or commit them to `.env`.

For a host-installed Caddy reverse proxy, adapt [`Caddyfile.example`](Caddyfile.example) to your DNS name. Caddy provisions HTTPS certificates automatically. Keep the management listener bound to loopback, expose only the proxy's HTTPS port, and restrict inbound network access. The sample is a starting point; validate TLS policy, DNS, certificate renewal, and firewall rules in the target environment.
