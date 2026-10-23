# SignalForge Collector deployment

This package runs the SignalForge OpenTelemetry Collector with Podman Compose
and persists received logs as JSON Lines. It is an early deployment package:
validate it on a test instance before relying on it for production data.

## Requirements

- Linux host with Podman and `podman-compose` (or compatible Compose)
- Permission to pull `ghcr.io/madirajupranaykashyap/signalforge-collector`
- Enough disk space for the archive and backups

## Install and start

Run from the repository root:

```bash
cd deploy/collector
cp .env.example .env
mkdir -p archive
podman-compose -f compose.yaml pull
podman-compose -f compose.yaml up -d
podman-compose -f compose.yaml ps
```

The default ports bind to loopback only:

- OTLP/gRPC: `127.0.0.1:4317`
- OTLP/HTTP: `127.0.0.1:4318`

The active archive is `deploy/collector/archive/logs.jsonl`. The Collector
currently appends to this single file so that data survives Collector restarts.
Automatic file rotation is deliberately not enabled: the upstream file exporter
does not support `append: true` together with rotation, and disabling append
can truncate the active file when the Collector restarts. Until a restart-safe
rotation mechanism is implemented, monitor disk usage and use an external
archive/retention process that is designed for an actively written file.

The volume uses Podman's `:Z` SELinux label. If your Compose implementation
does not support this suffix, use a compatible Podman Compose version or
configure the host SELinux label explicitly.

## Verify ingestion

Submit a sample log and confirm the marker was archived:

```bash
curl -i -X POST http://127.0.0.1:4318/v1/logs \
  -H 'Content-Type: application/json' \
  -d '{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"signalforge-verification"}}]},"scopeLogs":[{"logRecords":[{"body":{"stringValue":"SIGNALFORGE-VERIFY-001"},"severityText":"INFO"}]}]}]}'

grep -n 'SIGNALFORGE-VERIFY-001' archive/logs.jsonl
podman-compose -f compose.yaml logs --tail=100 collector
podman inspect --format '{{.State.Health.Status}}' signalforge-collector
```

A successful OTLP/HTTP request normally returns HTTP 200. The marker should
appear in the archive. The health endpoint is internal to the container.

## Configuration and network safety

Copy and edit `.env` before starting. Use `COLLECTOR_IMAGE` to pin a release
tag (for example `ghcr.io/madirajupranaykashyap/signalforge-collector:v0.1.0`)
or an immutable digest. Use `latest` only if you explicitly want to follow
the newest stable release. A tag is only usable after its GitHub release/tag
workflow has successfully published it.

To receive telemetry from another host, set `OTLP_GRPC_BIND` and/or
`OTLP_HTTP_BIND` to the host's specific LAN address and restrict access with
the host firewall and a trusted network. Do not expose OTLP ports to the public
internet without appropriate network controls and transport security.

The raw archive pipeline writes every received log once. A separate filtered
pipeline drops configured noisy records from the debug/downstream stream while
leaving the archive intact. The sample package does not send data to a vendor;
configure a real destination and credentials before adding an OTLP exporter.

## Upgrade and rollback

1. Back up the archive and your `.env` file.
2. Change `COLLECTOR_IMAGE` in `.env` to the exact release tag you want.
3. Pull and recreate the service:

   ```bash
   podman-compose -f compose.yaml pull
   podman-compose -f compose.yaml up -d
   podman-compose -f compose.yaml ps
   podman-compose -f compose.yaml logs --tail=100 collector
   ```

4. Send a test log and verify it is appended to the archive.

To roll back, set `COLLECTOR_IMAGE` to the previous known-good version, then
repeat the pull and recreate commands. Keep the previous image tag and archive
backup until the new version has passed verification. Do not use `latest` as
the rollback target because it moves.

## Archive retention and backup

The active archive is newline-delimited JSON and currently grows without
automatic rotation. Monitor disk usage and copy important archives off-host.
Stop or quiesce the collector before making a file-level backup to avoid
capturing a partially written final line. Back up the entire archive directory.
Do not configure the upstream file exporter with rotation while `append: true`
is required for restart durability. For example:

```bash
podman-compose -f compose.yaml stop collector
tar -czf signalforge-archive-$(date +%F).tar.gz archive
podman-compose -f compose.yaml start collector
```

Store backups off the host and test restoring them. Do not delete or rotate the
active file while the collector is writing to it unless you have configured
and verified a rotation strategy.

## Isolated test on Rocky Linux

Do not run this test beside an existing SmartHome-managed Collector using the
default ports/container name. To run an isolated copy, create a separate test
directory and environment file, set a unique container name, non-conflicting
ports, and a separate archive directory:

```bash
mkdir -p ~/signalforge-test/archive
cd ~/signalforge-test
cp /path/to/SignalForge-Community/deploy/collector/compose.yaml .
cat > .env <<'EOF'
COLLECTOR_IMAGE=ghcr.io/madirajupranaykashyap/signalforge-collector:latest
COLLECTOR_CONTAINER_NAME=signalforge-collector-test
OTLP_GRPC_BIND=127.0.0.1
OTLP_GRPC_PORT=14317
OTLP_HTTP_BIND=127.0.0.1
OTLP_HTTP_PORT=14318
ARCHIVE_DIR=./archive
EOF
podman-compose -p signalforge-test -f compose.yaml pull
podman-compose -p signalforge-test -f compose.yaml up -d
podman-compose -p signalforge-test -f compose.yaml ps
curl -i -X POST http://127.0.0.1:14318/v1/logs \
  -H 'Content-Type: application/json' \
  -d '{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"rocky-smoke-test"}}]},"scopeLogs":[{"logRecords":[{"body":{"stringValue":"SIGNALFORGE-ROCKY-VERIFY"},"severityText":"INFO"}]}]}]}'
grep -n 'SIGNALFORGE-ROCKY-VERIFY' archive/logs.jsonl
```

Replace `/path/to/SignalForge-Community` with your actual checkout path. This
test uses ports 14317/14318 and its own archive, so it should not disturb the
existing SmartHome Collector. If the image is private, log in to GHCR first.
After verification, remove only the test project with
`podman-compose -p signalforge-test -f compose.yaml down`; keep the archive
until you have decided whether to retain it.

## Avoid duplicate collectors

If your SmartHome Podman Compose stack already runs a collector, do not start
the standalone package with the default settings. Both would otherwise compete
for host ports 4317/4318. Keep the existing service or use the isolated test
settings above.
