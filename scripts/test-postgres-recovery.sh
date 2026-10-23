#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source_url=${SIGNALFORGE_TEST_POSTGRES_URL:-}
target_url=${SIGNALFORGE_TEST_POSTGRES_RESTORE_URL:-}
[[ -n "$source_url" && -n "$target_url" ]] || { echo "PostgreSQL test URLs are required" >&2; exit 2; }
for tool in age age-keygen pg_dump pg_restore psql; do
  command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
done
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
identity="$tmp/identity.txt"
age-keygen -o "$identity" 2>/dev/null
recipient=$(age-keygen -y "$identity")
psql "$source_url" --no-psqlrc --set ON_ERROR_STOP=1 --command "DROP TABLE IF EXISTS signalforge_recovery_probe; CREATE TABLE signalforge_recovery_probe(value TEXT NOT NULL); INSERT INTO signalforge_recovery_probe VALUES ('encrypted-recovery-marker');"
backup=$("$root/scripts/management-backup-postgres-encrypted.sh" "$source_url" "$recipient" "$tmp/backups")
[[ -s "$backup" ]]
if grep --binary-files=text -q 'encrypted-recovery-marker' "$backup"; then
  echo "FAIL: backup appears to contain plaintext database content" >&2
  exit 1
fi
"$root/scripts/management-restore-postgres-encrypted.sh" "$target_url" "$backup" "$identity"
value=$(psql "$target_url" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 --command "SELECT value FROM signalforge_recovery_probe")
[[ "$value" == "encrypted-recovery-marker" ]] || { echo "FAIL: restored marker did not match" >&2; exit 1; }

# A non-empty target must not be overwritten.
if "$root/scripts/management-restore-postgres-encrypted.sh" "$target_url" "$backup" "$identity" >/dev/null 2>&1; then
  echo "FAIL: restore accepted a non-empty target database" >&2
  exit 1
fi
echo "PASS: PostgreSQL encrypted backup/restore and overwrite guard"
