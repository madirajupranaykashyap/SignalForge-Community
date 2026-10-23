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

The raw archive is `archive/logs.jsonl`; the optimized downstream stream is
also written to `archive/optimized.jsonl` so record and byte trends can be
compared. Both files use append mode for restart durability. Do not configure
the upstream file exporter to rotate these files itself. Use the supplied
`scripts/archive-rotate.sh` helper, which stops the Collector before moving
the active files, restarts it to reopen fresh files, compresses rotated files,
and prunes compressed archives by age. Defaults are 256 MiB per active file
and 14 days of compressed retention. Run it from a scheduled job (for example,
a systemd timer) and monitor free disk space; rotation is not automatically
scheduled by Compose.

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

The raw archive pipeline writes every received log before optimization. The
optimized stream applies deterministic environment-configured equality rules
and writes accepted records to both the debug exporter and
`archive/optimized.jsonl`. Traces and metrics are accepted by inspection
pipelines and sent to the debug exporter; they are **not persisted** to these
log archives. The sample package does not send data to a vendor.

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

Run the rotation helper from a scheduler appropriate to your host. It rotates
both raw and optimized streams only after either active file reaches the size
threshold, stops the Collector before moving active files, starts it so the
append-mode exporters reopen new files, compresses the closed files, and removes
compressed archives older than the retention window.

```bash
ARCHIVE_MAX_MB=256 ARCHIVE_RETENTION_DAYS=14 scripts/archive-rotate.sh
```

Set `ARCHIVE_DIR` if your archive volume is mounted elsewhere. The script
expects `podman-compose` and the deployment's `compose.yaml`; override
`COMPOSE_CMD` or `COMPOSE_FILE` when your environment uses a different
compatible command/path. It is safe to run periodically: below the threshold it
does not stop the Collector. Rotation itself is not scheduled automatically.
Monitor free disk space and schedule backups separately. For a consistent
backup, stop or quiesce the Collector, copy the whole archive directory, and
test restoration.

## Isolated test on Rocky Linux

Do not run this test beside an existing SmartHome-managed Collector using the
default ports/container name. To run an isolated copy, create a separate test
directory and environment file, set a unique container name, non-conflicting
ports, and a separate archive directory:

```bash
mkdir -p ~/signalforge-test/archive ~/signalforge-test/metrics
cd ~/signalforge-test
cp /path/to/SignalForge-Community/deploy/collector/compose.yaml .
cp /path/to/SignalForge-Community/deploy/collector/metrics/archive_metrics.py ./metrics/
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


## Optimization measurements

The Collector emits separate input and optimized record counters. The
`archive-metrics` sidecar exposes active-file byte gauges and incremental line
counters at `http://127.0.0.1:9465/metrics` (host port configurable with
`ARCHIVE_METRICS_PORT`). It reports raw and optimized archive file sizes and
record counts. These are operational measurements, not a full cost model:
file bytes do not include network, storage compression, backend indexing, or
retention costs, and Collector CPU/latency overhead is not measured yet.
Recording rules for Collector record rates and record reduction remain in
`prometheus/recording-rules.yml`.


## Prometheus optimization metrics

The Collector exposes optimization count metrics on a Prometheus-compatible
`/metrics` endpoint at container port `9464`. Compose publishes this on
`127.0.0.1:9464` by default; use `METRICS_BIND` and `METRICS_PORT` to
change the host binding. Keep it on loopback unless your monitoring network is
trusted and access is restricted by firewall policy.

Expected metric names include `signalforge_input_log_records` and
`signalforge_optimized_log_records` (Prometheus normalizes dots to underscores).
Scrape the endpoint and calculate record reduction over the same interval.
These counters measure record counts, not byte savings or monetary savings.


## Optimization analytics recording rules

Load `prometheus/recording-rules.yml` from your Prometheus configuration using
`rule_files`. The rules run every 30 seconds and calculate five-minute rates
for input and optimized log records, plus the record-reduction percentage:

```promql
100 * (1 - rate(signalforge_optimized_log_records[5m])
           / clamp_min(rate(signalforge_input_log_records[5m]), 1e-9))
```

The recording-rule file aggregates across matching counter series. Query the
recorded series `signalforge:input_log_records:rate5m`,
`signalforge:optimized_log_records:rate5m`, and
`signalforge:log_record_reduction_percent:rate5m` in dashboards. The reduction
percentage can be negative if optimized throughput exceeds input throughput
within the selected rate window; investigate that rather than interpreting it
as savings. During idle periods the rate-based percentage can be noisy.

These are **record-count** analytics only. They do not establish bytes saved,
CPU or latency overhead, policy-specific drop reasons, or monetary savings.
Those require additional instrumentation and a defined cost model.

## Grafana dashboard

Import `grafana/signalforge-optimization-dashboard.json` into Grafana and
select the Prometheus data source that scrapes the Collector's `/metrics`
endpoint. The dashboard uses the recording-rule series defined in
`prometheus/recording-rules.yml`, so configure and validate those rules first.

The dashboard shows input and optimized log-record rates, their comparison,
and the calculated record-count reduction percentage. A negative percentage
means optimized throughput exceeded input throughput during the selected rate
window; investigate the counters and scrape coverage. Sparse traffic can make
the percentage noisy. These charts do not estimate payload bytes, infrastructure
cost, or money saved.

## Prometheus configuration and alerting

Start with `prometheus/prometheus.yml.example` and adjust the scrape target to an
address reachable from the Prometheus process. The example loads both
`prometheus/recording-rules.yml` and `prometheus/alerting-rules.yml`; the
scrape job name must remain `signalforge-collector` unless you also update the
alert expressions. Validate configuration and rules before reloading Prometheus:

```bash
promtool check config /path/to/prometheus.yml
promtool check rules deploy/collector/prometheus/recording-rules.yml
promtool check rules deploy/collector/prometheus/alerting-rules.yml
```

The included alert rules cover three operational signals:

- **Collector scrape missing**: Prometheus reports the Collector target down for
  two minutes.
- **No input log records**: the Collector is scrapeable, but the input counter
  is absent for ten minutes. This can be expected when the deployment is idle;
  tune or disable this alert if zero traffic is normal.
- **Negative record reduction**: optimized throughput exceeds input throughput
  for ten minutes. Investigate scrape coverage, counter resets, and pipeline
  topology rather than interpreting this as savings.

The Collector metrics endpoint defaults to loopback. Configure Prometheus on the
same host or provide a deliberately restricted, reachable network path; do not
expose the endpoint publicly. The dashboard and alerts depend on the recording
rules being loaded and evaluated. CI validates the Prometheus configuration,
recording/alerting rule syntax, and dashboard JSON structure, in addition to the
existing Collector build and ingestion smoke tests. Live Grafana rendering and
production alert routing still need to be verified in the target environment.



## Phases 12–16: operating the Collector safely

### Configurable policy

Set these values in `.env` before starting the stack:

- `SIGNALFORGE_DROP_SEVERITY` (default `DEBUG`)
- `SIGNALFORGE_HEALTH_ROUTE` (default `/health`)
- `SIGNALFORGE_HEALTH_SERVICE` (default `health-check`)

Each value is used by one exact-match filter condition. To disable one equality
condition, set its value to `__disabled__` (provided that sentinel is not a
real value in your telemetry). These are simple equality rules, not arbitrary
user-supplied expressions. Review policy changes in a test deployment before
promoting them; the raw archive intentionally retains records dropped from the
optimized stream. Restart/recreate the Collector after changing environment
values. Keep a record of the policy values used for each deployment.

### Archive metrics

Prometheus can scrape the archive sidecar separately from the Collector:
`127.0.0.1:9465/metrics` by default. Useful series:

- `signalforge_archive_file_bytes{stream="raw|optimized"}`
- `signalforge_archive_file_present{stream="raw|optimized"}`
- `signalforge_archive_records_total{stream="raw|optimized"}`

The sidecar reads the active files incrementally and detects file replacement,
truncation, and missing files. Its record counters describe the current active
file and reset when the file rotates; use them as active-file diagnostics, not
as a lifetime total. Restrict the endpoint to a trusted monitoring network.
Do not infer monetary savings from file-size differences alone.

### Reliability and security checklist

- Keep OTLP and both metrics ports loopback-bound unless you intentionally
  configure a protected LAN path and firewall rules.
- Pin the Collector image to a release tag or digest in production; do not rely
  on a moving `latest` tag for reproducible rollback.
- Keep the container non-root and preserve the health check and graceful
  shutdown period. Verify archive persistence after a restart.
- Test low disk space, unavailable downstream exporters, and abrupt host
  shutdown in a disposable environment before production use. A disk-full event
  can still cause archive writes to fail; alert on disk utilization and archive
  health at the host level.
- Back up both raw and optimized archives, and test restore procedures.

### Integration coverage and current limitations

CI exercises OTLP/HTTP ingestion for logs, traces, and metrics; raw archive
persistence; DEBUG filtering; optimized output; archive metric calculations;
and Prometheus rule/dashboard validation. Traces and metrics still go to the
debug exporter only. This phase does not add vendor exporters, adaptive
sampling, a monetary cost model, Collector CPU/latency profiling, or automated
rotation scheduling. Those remain separate future work.


## Remote configuration agent

The image now starts `signalforge-agent`, which supervises the bundled `signalforge-collector`. With no remote-config variables set, it starts the baked-in local configuration and continues to process telemetry locally. To enable polling, set both:

- `SIGNALFORGE_CONFIG_ENDPOINT`: full HTTPS URL ending in `/api/v1/agents/config`.
- `SIGNALFORGE_AGENT_CREDENTIAL`: one-time-enrollment credential returned by the management API (store it in a secret manager; do not commit it to `.env`).

The agent polls every `SIGNALFORGE_POLL_INTERVAL` (default `30s`). It validates the received JSON/YAML-compatible Collector config using the bundled Collector binary, stages the candidate under `/var/lib/signalforge/config`, restarts the Collector, and checks its health endpoint. If validation, restart, or health checks fail, it restores the previous configuration and acknowledges rejection. The last-known-good cache and active config live in a persistent named volume, `signalforge-config`.

Remote endpoints must use HTTPS; HTTP is accepted only for loopback development. Control-plane outages use bounded retry backoff and do not intentionally stop the currently running Collector pipeline. Verify this behavior with the target environment before production use.
