
#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BINARY="$ROOT/collector/build/signalforge-collector"
CONFIG="$ROOT/collector/config/local.yaml"
LOG_FILE="$(mktemp)"
METRICS_FILE="$(mktemp)"

cleanup() {
  if [[ -n "${COLLECTOR_PID:-}" ]]; then
    kill "$COLLECTOR_PID" 2>/dev/null || true
    wait "$COLLECTOR_PID" 2>/dev/null || true
  fi
  rm -f "$LOG_FILE" "$METRICS_FILE"
}
trap cleanup EXIT

if ! [[ -x "$BINARY" ]]; then
  echo "Collector binary not found or not executable: $BINARY" >&2
  exit 1
fi

echo "Starting SignalForge Collector..."
"$BINARY" --config="$CONFIG" >"$LOG_FILE" 2>&1 &
COLLECTOR_PID=$!

ready=0
for _ in $(seq 1 30); do
  if ! kill -0 "$COLLECTOR_PID" 2>/dev/null; then
    echo "Collector exited unexpectedly:"
    cat "$LOG_FILE"
    exit 1
  fi

  if curl -fsS http://127.0.0.1:8888/metrics >/dev/null 2>&1 &&
     curl -sS -o /dev/null \
       -w '%{http_code}' \
       http://127.0.0.1:4318/ \
       2>/dev/null | grep -qE '^[2345][0-9][0-9]$'; then
    ready=1
    break
  fi

  sleep 1
done

if [[ "$ready" != 1 ]]; then
  echo "Collector endpoints did not become ready."
  cat "$LOG_FILE"
  exit 1
fi

echo "Sending two test log records..."

curl -fsS http://127.0.0.1:4318/v1/logs \
  -H 'Content-Type: application/json' \
  -d '{
    "resourceLogs": [{
      "scopeLogs": [{
        "logRecords": [
          {"body": {"stringValue": "HEALTH_CHECK"}},
          {
            "severityText": "ERROR",
            "body": {"stringValue": "Database connection failed"}
          }
        ]
      }]
    }]
  }' >/dev/null

# Allow the batch processor to flush.
sleep 2

if ! grep -Fq 'Database connection failed' "$LOG_FILE"; then
  echo "FAIL: Database error was not forwarded."
  cat "$LOG_FILE"
  exit 1
fi

if grep -Fq 'HEALTH_CHECK' "$LOG_FILE"; then
  echo "FAIL: Health-check log appeared in exported output."
  cat "$LOG_FILE"
  exit 1
fi

curl -fsS http://127.0.0.1:8888/metrics >"$METRICS_FILE"

get_counter() {
  local metric="$1"
  local labels="$2"

  awk -v metric="$metric" -v labels="$labels" '
    index($0, metric "{" labels "}") == 1 {
      print $NF
      found = 1
      exit
    }
    END {
      if (!found) exit 1
    }
  ' "$METRICS_FILE"
}

accepted="$(get_counter \
  otelcol_receiver_accepted_log_records \
  'receiver="otlp",transport="http"')"

dropped="$(get_counter \
  otelcol_processor_filter_logs_filtered \
  'filter="filter"')"

forwarded="$(get_counter \
  otelcol_exporter_sent_log_records \
  'exporter="debug"')"

if [[ "$accepted" != "2" || "$dropped" != "1" || "$forwarded" != "1" ]]; then
  echo "FAIL: Unexpected counter values."
  echo "Accepted=$accepted, dropped=$dropped, forwarded=$forwarded"
  exit 1
fi

reduction="$(awk -v input="$accepted" -v output="$forwarded" \
  'BEGIN {
    if (input == 0) print "0.00"
    else printf "%.2f", (input-output)*100/input
  }')"

echo "PASS: Database error was preserved."
echo "PASS: Health-check log was filtered."
echo "PASS: Actual Collector counters match expectations."
echo "Accepted: $accepted"
echo "Dropped: $dropped"
echo "Forwarded: $forwarded"
echo "Record-count reduction: ${reduction}%"