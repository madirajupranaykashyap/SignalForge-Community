# SignalForge Collector deployment

This package runs the SignalForge OpenTelemetry Collector with Podman Compose
and persists received logs as JSON Lines.

## Requirements

- Podman and `podman-compose` (or a compatible Compose implementation)
- Access to pull `ghcr.io/madirajupranaykashyap/signalforge-collector`
- Linux host with sufficient disk space for the archive

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

The default published ports are bound to `127.0.0.1` only:

- OTLP/gRPC: `127.0.0.1:4317`
- OTLP/HTTP: `127.0.0.1:4318`

The archive is written to `deploy/collector/archive/logs.jsonl`. The volume
uses the Podman `:Z` SELinux label for a private container mount. If your
Compose implementation does not support this suffix, use a compatible Podman
Compose version or configure the host SELinux label explicitly.

## Verify log ingestion

With the collector running, submit a test log over OTLP/HTTP:

```bash
curl -i -X POST http://127.0.0.1:4318/v1/logs \
  -H 'Content-Type: application/json' \
  -d '{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"signalforge-verification"}}]},"scopeLogs":[{"logRecords":[{"body":{"stringValue":"SIGNALFORGE-VERIFY-001"},"severityText":"INFO"}]}]}]}'

grep -n 'SIGNALFORGE-VERIFY-001' archive/logs.jsonl
```

A successful OTLP/HTTP request normally returns HTTP 200. The marker should
appear once in the archive. Check logs and health status with:

```bash
podman-compose -f compose.yaml logs --tail=100 collector
podman inspect --format '{{.State.Health.Status}}' signalforge-collector
```

## Configuration

Copy and edit `.env` before starting. You can set `COLLECTOR_IMAGE` to a
versioned tag or immutable digest rather than `latest`. To receive telemetry
from another host, set `OTLP_GRPC_BIND` and/or `OTLP_HTTP_BIND` to a specific
host LAN address, then restrict access with the host firewall and a trusted
network. Do not expose OTLP ports to the public internet without suitable
network controls and transport security.

The raw archive pipeline writes every received log once. A separate filtered
pipeline drops configured noisy records from the debug/downstream stream while
leaving the archive intact. The sample package does not send data to a vendor;
configure an actual destination and credentials before adding an OTLP exporter
to the filtered pipeline.

## Important: avoid running two collectors on the same ports

If your SmartHome Podman Compose stack already runs this collector, do not
start this standalone package at the same time. Both deployments use host
ports 4317/4318 and the container name `signalforge-collector`. Either keep
the existing SmartHome-managed collector and use its configuration, or stop
and remove that service from the SmartHome stack before starting this package.
