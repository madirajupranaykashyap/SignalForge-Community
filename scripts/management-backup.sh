#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 DATABASE_PATH BACKUP_DIRECTORY" >&2
  exit 2
}
[[ $# -eq 2 ]] || usage
db=$1
out_dir=$2
command -v sqlite3 >/dev/null || { echo "sqlite3 is required" >&2; exit 1; }
[[ -f "$db" ]] || { echo "Database not found: $db" >&2; exit 1; }
mkdir -p "$out_dir"
chmod 700 "$out_dir"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
dest="$out_dir/management-$stamp.db"
tmp="$dest.tmp"
trap 'rm -f "$tmp"' EXIT

# SQLite's dot-command accepts a quoted filename; reject apostrophes rather
# than risk creating a backup at an unintended path.
[[ "$tmp" != *"'"* ]] || { echo "Backup path must not contain apostrophes" >&2; exit 1; }
sqlite3 "$db" ".timeout 10000" ".backup '$tmp'"
check=$(sqlite3 "$tmp" 'PRAGMA integrity_check;')
[[ "$check" == "ok" ]] || { echo "Backup integrity check failed: $check" >&2; exit 1; }
chmod 600 "$tmp"
mv "$tmp" "$dest"
trap - EXIT
echo "$dest"
