#!/usr/bin/env bash
set -euo pipefail

ARCHIVE_FILE="data/archive/logs.jsonl"
ENDPOINT="http://127.0.0.1:4318/v1/logs"

mkdir -p "$(dirname "$ARCHIVE_FILE")"

# Unique markers prevent records from earlier test runs affecting counts.
RUN_ID="$(date +%s)_$$"
HEALTH_MARKER="SF_HEALTH_CHECK_${RUN_ID}"
ERROR_MARKER="SF_DATABASE_ERROR_${RUN_ID}"

echo "Sending two test records to SignalForge..."

curl -fsS "$ENDPOINT" \
  -H 'Content-Type: application/json' \
  -d "{
    \"resourceLogs\": [{
      \"scopeLogs\": [{
        \"scope\": {\"name\": \"signalforge-archive-test\"},
        \"logRecords\": [
          {\"body\": {\"stringValue\": \"$HEALTH_MARKER\"}},
          {
            \"severityText\": \"ERROR\",
            \"body\": {\"stringValue\": \"$ERROR_MARKER\"}
          }
        ]
      }]
    }]
  }" >/dev/null

for _ in $(seq 1 20); do
  if [[ -f "$ARCHIVE_FILE" ]] &&
     grep -qF "$HEALTH_MARKER" "$ARCHIVE_FILE" &&
     grep -qF "$ERROR_MARKER" "$ARCHIVE_FILE"; then
    break
  fi
  sleep 1
done

if [[ ! -f "$ARCHIVE_FILE" ]]; then
  echo "FAIL: Archive file was not created."
  exit 1
fi

health_count=$(grep -oF "$HEALTH_MARKER" "$ARCHIVE_FILE" | wc -l | tr -d ' ')
error_count=$(grep -oF "$ERROR_MARKER" "$ARCHIVE_FILE" | wc -l | tr -d ' ')

if [[ "$health_count" -ne 1 ]]; then
  echo "FAIL: Expected 1 archived health-check record; found $health_count."
  exit 1
fi

if [[ "$error_count" -ne 1 ]]; then
  echo "FAIL: Expected 1 archived database-error record; found $error_count."
  exit 1
fi

echo "PASS: Health-check record exists in the archive."
echo "PASS: Database-error record exists in the archive."
echo "PASS: Both records were archived independently of the vendor filter."
echo "Archive completeness: 2/2 expected records found."