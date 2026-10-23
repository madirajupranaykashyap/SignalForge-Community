#!/bin/sh
# Restart-safe archive rotation for append-mode Collector file exporters.
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE_FILE=${COMPOSE_FILE:-"$SCRIPT_DIR/compose.yaml"}
ARCHIVE_DIR=${ARCHIVE_DIR:-"$SCRIPT_DIR/archive"}
ARCHIVE_MAX_MB=${ARCHIVE_MAX_MB:-256}
ARCHIVE_RETENTION_DAYS=${ARCHIVE_RETENTION_DAYS:-14}
COMPOSE_CMD=${COMPOSE_CMD:-podman-compose}

case "$ARCHIVE_MAX_MB:$ARCHIVE_RETENTION_DAYS" in
  *[!0-9:]*|:*|*:) echo "ARCHIVE_MAX_MB and ARCHIVE_RETENTION_DAYS must be non-negative integers." >&2; exit 2 ;;
esac
[ "$ARCHIVE_MAX_MB" -gt 0 ] || { echo "ARCHIVE_MAX_MB must be greater than zero." >&2; exit 2; }
[ -d "$ARCHIVE_DIR" ] || { echo "Archive directory not found: $ARCHIVE_DIR" >&2; exit 2; }

max_bytes=$((ARCHIVE_MAX_MB * 1024 * 1024))
should_rotate=false
for stream in logs optimized; do
  active="$ARCHIVE_DIR/$stream.jsonl"
  if [ -f "$active" ]; then
    size=$(wc -c < "$active" | tr -d ' ')
    if [ "$size" -ge "$max_bytes" ]; then should_rotate=true; fi
  fi
done

if [ "$should_rotate" != true ]; then
  echo "Archive rotation not needed (threshold: ${ARCHIVE_MAX_MB} MiB)."
  exit 0
fi

timestamp=$(date -u +%Y%m%dT%H%M%SZ)
stopped=false
restart_collector() {
  if [ "$stopped" = true ]; then
    "$COMPOSE_CMD" -f "$COMPOSE_FILE" start collector
    stopped=false
  fi
}
trap 'restart_collector' EXIT HUP INT TERM

"$COMPOSE_CMD" -f "$COMPOSE_FILE" stop collector
stopped=true

for stream in logs optimized; do
  active="$ARCHIVE_DIR/$stream.jsonl"
  if [ -f "$active" ] && [ -s "$active" ]; then
    mv "$active" "$ARCHIVE_DIR/$stream-$timestamp.jsonl"
  fi
done

restart_collector
trap - EXIT HUP INT TERM

for rotated in "$ARCHIVE_DIR"/logs-*.jsonl "$ARCHIVE_DIR"/optimized-*.jsonl; do
  [ -f "$rotated" ] || continue
  gzip -n "$rotated"
done

find "$ARCHIVE_DIR" -type f \( -name 'logs-*.jsonl.gz' -o -name 'optimized-*.jsonl.gz' \) \
  -mtime "+$ARCHIVE_RETENTION_DAYS" -delete

echo "Rotated archives at $timestamp; retained compressed archives for $ARCHIVE_RETENTION_DAYS days."
