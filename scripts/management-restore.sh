#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 BACKUP_PATH DATABASE_PATH" >&2
  echo "Stop every management API writer before running restore." >&2
  exit 2
}
[[ $# -eq 2 ]] || usage
backup=$1
db=$2
command -v sqlite3 >/dev/null || { echo "sqlite3 is required" >&2; exit 1; }
[[ -f "$backup" ]] || { echo "Backup not found: $backup" >&2; exit 1; }
check=$(sqlite3 "$backup" 'PRAGMA integrity_check;')
[[ "$check" == "ok" ]] || { echo "Backup integrity check failed: $check" >&2; exit 1; }

dir=$(dirname "$db")
mkdir -p "$dir"
# Build the replacement beside the target so rename is atomic on this filesystem.
tmp=$(mktemp "$dir/.management-restore.XXXXXX")
trap 'rm -f "$tmp"' EXIT
cp "$backup" "$tmp"
chmod 600 "$tmp"
check=$(sqlite3 "$tmp" 'PRAGMA integrity_check;')
[[ "$check" == "ok" ]] || { echo "Restored copy integrity check failed: $check" >&2; exit 1; }

if [[ -e "$db" ]]; then
  echo "Refusing to overwrite existing database: $db" >&2
  echo "Move it aside after stopping the service, then rerun restore." >&2
  exit 1
fi
mv "$tmp" "$db"
trap - EXIT
echo "Restored database to $db"
