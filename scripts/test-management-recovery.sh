#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
backup_script="$root/scripts/management-backup.sh"
restore_script="$root/scripts/management-restore.sh"
command -v sqlite3 >/dev/null || { echo "SKIP: sqlite3 not installed"; exit 0; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
db="$tmp/source.db"
restore="$tmp/restored.db"
out="$tmp/backups"
mkdir -p "$out"
sqlite3 "$db" 'CREATE TABLE sample (id INTEGER PRIMARY KEY, value TEXT NOT NULL); INSERT INTO sample(value) VALUES ("persisted");'

created=$("$backup_script" "$db" "$out")
[[ -f "$created" ]]
[[ "$(sqlite3 "$created" 'PRAGMA integrity_check;')" == "ok" ]]
[[ "$(sqlite3 "$created" 'SELECT value FROM sample WHERE id = 1;')" == "persisted" ]]

"$restore_script" "$created" "$restore"
[[ "$(sqlite3 "$restore" 'SELECT value FROM sample WHERE id = 1;')" == "persisted" ]]

# Restoring over an existing DB must fail and leave its contents unchanged.
if "$restore_script" "$created" "$restore" >/dev/null 2>&1; then
  echo "FAIL: restore overwrote an existing database" >&2
  exit 1
fi
[[ "$(sqlite3 "$restore" 'SELECT value FROM sample WHERE id = 1;')" == "persisted" ]]

# Corrupt/invalid input must be rejected before a target is created.
printf 'not a sqlite database\n' > "$tmp/corrupt.db"
if "$restore_script" "$tmp/corrupt.db" "$tmp/should-not-exist.db" >/dev/null 2>&1; then
  echo "FAIL: restore accepted an invalid backup" >&2
  exit 1
fi
[[ ! -e "$tmp/should-not-exist.db" ]]

echo "PASS: management backup/restore smoke tests"
