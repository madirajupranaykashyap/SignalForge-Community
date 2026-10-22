#!/usr/bin/env bash
set -euo pipefail

ENDPOINT="http://127.0.0.1:4318/v1/logs"
METRICS_ENDPOINT="http://127.0.0.1:8888/metrics"
ARCHIVE_FILE="data/archive/logs.jsonl"

TOTAL=100
FILTERED_EXPECTED=50
VENDOR_EXPECTED=50
RUN_ID="$(date +%s)_$$"
MARKER_PREFIX="SF_RUN_${RUN_ID}_"

PAYLOAD="$(mktemp)"
VENDOR_PAYLOAD="$(mktemp)"

trap 'rm -f "$PAYLOAD" "$VENDOR_PAYLOAD"' EXIT

mkdir -p "$(dirname "$ARCHIVE_FILE")"

if ! curl -fsS "$METRICS_ENDPOINT" >/dev/null; then
  echo "FAIL: Collector metrics endpoint unavailable: $METRICS_ENDPOINT"
  exit 1
fi

metric_value() {
  local metric="$1"
  local label1="${2:-}"
  local label2="${3:-}"

  curl -fsS "$METRICS_ENDPOINT" |
    awk -v prefix="${metric}{" -v l1="$label1" -v l2="$label2" '
      index($0, prefix) == 1 &&
      (l1 == "" || index($0, l1) > 0) &&
      (l2 == "" || index($0, l2) > 0) {
        sum += $NF
      }
      END { printf "%.0f\n", sum }
    '
}

echo "Preparing $TOTAL test records..."

RUN_ID="$RUN_ID" TOTAL="$TOTAL" python3 - "$PAYLOAD" "$VENDOR_PAYLOAD" <<'PY'
import json
import os
import sys

run_id = os.environ["RUN_ID"]
total = int(os.environ["TOTAL"])

all_records = []
vendor_records = []

for i in range(total):
    record = {
        "attributes": [{
            "key": "sf.benchmark_id",
            "value": {"stringValue": f"SF_RUN_{run_id}_{i}"}
        }],
        "body": {
            "stringValue": (
                "HEALTH_CHECK"
                if i < total // 2
                else "Database connection failed"
            )
        }
    }

    if i >= total // 2:
        record["severityText"] = "ERROR"
        vendor_records.append(record)

    all_records.append(record)

def make_payload(records):
    return {
        "resourceLogs": [{
            "resource": {},
            "scopeLogs": [{
                "scope": {"name": "signalforge-benchmark"},
                "logRecords": records
            }]
        }]
    }

for path, records in [
    (sys.argv[1], all_records),
    (sys.argv[2], vendor_records)
]:
    with open(path, "w", encoding="utf-8") as f:
        json.dump(make_payload(records), f, separators=(",", ":"))
PY

# Measure compact OTLP JSON request sizes.
input_bytes=$(wc -c < "$PAYLOAD" | tr -d ' ')
vendor_bytes=$(wc -c < "$VENDOR_PAYLOAD" | tr -d ' ')

if (( input_bytes > 0 )); then
  bytes_saved=$((input_bytes - vendor_bytes))
  byte_reduction=$((bytes_saved * 100 / input_bytes))
else
  echo "FAIL: Input payload is empty."
  exit 1
fi

# Snapshot cumulative Collector counters before this run.
received_before=$(metric_value \
  otelcol_receiver_accepted_log_records 'receiver="otlp"' 'transport="http"')
filtered_before=$(metric_value \
  otelcol_processor_filter_logs_filtered 'filter="filter"')
sent_before=$(metric_value \
  otelcol_exporter_sent_log_records 'exporter="debug"')

echo "Sending $TOTAL records to SignalForge..."

curl -fsS "$ENDPOINT" \
  -H 'Content-Type: application/json' \
  --data-binary "@$PAYLOAD" >/dev/null

# Wait until every unique test record is archived.
archive_count=0

for _ in $(seq 1 30); do
  if [[ -f "$ARCHIVE_FILE" ]]; then
    archive_count=$(
      grep -oF "$MARKER_PREFIX" "$ARCHIVE_FILE" |
        wc -l | tr -d ' ' || true
    )
    archive_count="${archive_count:-0}"
  fi

  if [[ "$archive_count" -ge "$TOTAL" ]]; then
    break
  fi

  sleep 1
done

received_after=$(metric_value \
  otelcol_receiver_accepted_log_records 'receiver="otlp"' 'transport="http"')
filtered_after=$(metric_value \
  otelcol_processor_filter_logs_filtered 'filter="filter"')
sent_after=$(metric_value \
  otelcol_exporter_sent_log_records 'exporter="debug"')

received=$((received_after - received_before))
filtered=$((filtered_after - filtered_before))
vendor=$((sent_after - sent_before))

echo
echo "========== SignalForge Benchmark =========="
printf "Records submitted:              %d\n" "$TOTAL"
printf "Records accepted (delta):       %d\n" "$received"
printf "Records filtered:               %d\n" "$filtered"
printf "Records sent to debug:          %d\n" "$vendor"
printf "Records archived:               %d\n" "$archive_count"
echo
echo "----- Payload size estimate -----"
printf "Original OTLP JSON payload:     %d bytes\n" "$input_bytes"
printf "Filtered OTLP JSON equivalent:  %d bytes\n" "$vendor_bytes"
printf "Estimated payload bytes saved:  %d bytes\n" "$bytes_saved"
printf "Estimated byte reduction:       %d%%\n" "$byte_reduction"
echo "==========================================="

if [[ "$received" -ne "$TOTAL" ||
      "$filtered" -ne "$FILTERED_EXPECTED" ||
      "$vendor" -ne "$VENDOR_EXPECTED" ||
      "$archive_count" -ne "$TOTAL" ]]; then
  echo "FAIL: Record counts differ from expectations."
  exit 1
fi

if (( vendor_bytes > input_bytes )); then
  echo "FAIL: Filtered payload is larger than original payload."
  exit 1
fi

echo "PASS: Filtering and archive completeness verified."
echo "PASS: Payload-size comparison calculated."
echo "NOTE: Byte savings are an estimate, not actual vendor wire bytes."

# Save a machine-readable benchmark report.
REPORT_DIR="reports/benchmarks"
REPORT_FILE="${REPORT_DIR}/signalforge-benchmark-${RUN_ID}.json"
GENERATED_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

mkdir -p "$REPORT_DIR"

RUN_ID="$RUN_ID" \
GENERATED_AT="$GENERATED_AT" \
TOTAL="$TOTAL" \
RECEIVED="$received" \
FILTERED="$filtered" \
VENDOR="$vendor" \
ARCHIVED="$archive_count" \
INPUT_BYTES="$input_bytes" \
VENDOR_BYTES="$vendor_bytes" \
BYTES_SAVED="$bytes_saved" \
python3 - "$REPORT_FILE" <<'PY'
import json
import os
import sys

input_bytes = int(os.environ["INPUT_BYTES"])
vendor_bytes = int(os.environ["VENDOR_BYTES"])
bytes_saved = int(os.environ["BYTES_SAVED"])
received = int(os.environ["RECEIVED"])
filtered = int(os.environ["FILTERED"])

report = {
    "schema_version": "1.0",
    "run_id": os.environ["RUN_ID"],
    "generated_at_utc": os.environ["GENERATED_AT"],
    "results": {
        "records_submitted": int(os.environ["TOTAL"]),
        "records_received": received,
        "records_filtered": filtered,
        "records_sent_to_debug_exporter": int(os.environ["VENDOR"]),
        "records_archived": int(os.environ["ARCHIVED"]),
        "record_reduction_percent": (
            round(filtered * 100 / received, 2) if received else None
        ),
        "original_otlp_json_bytes": input_bytes,
        "filtered_otlp_json_equivalent_bytes": vendor_bytes,
        "estimated_bytes_saved": bytes_saved,
        "estimated_byte_reduction_percent": (
            round(bytes_saved * 100 / input_bytes, 2)
            if input_bytes else None
        )
    },
    "measurement": {
        "vendor_wire_bytes_measured": False,
        "monetary_savings_calculated": False,
        "note": (
            "Filtered payload size is an equivalent OTLP JSON estimate. "
            "It is not measured vendor wire traffic or confirmed billing savings."
        )
    }
}

with open(sys.argv[1], "w", encoding="utf-8") as f:
    json.dump(report, f, indent=2)
    f.write("\n")

print(f"Benchmark report saved: {sys.argv[1]}")
PY