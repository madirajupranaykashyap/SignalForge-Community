#!/usr/bin/env bash
set -euo pipefail

usage() { echo "Usage: $0 EMPTY_TARGET_DATABASE_URL ENCRYPTED_BACKUP AGE_IDENTITY_FILE" >&2; exit 2; }
[[ $# -eq 3 ]] || usage
database_url=$1
backup=$2
identity=$3
for tool in age pg_restore psql; do
  command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
done
[[ "$database_url" == postgres://* || "$database_url" == postgresql://* ]] || { echo "DATABASE_URL must use postgres:// or postgresql://" >&2; exit 2; }
[[ -f "$backup" ]] || { echo "Encrypted backup not found: $backup" >&2; exit 1; }
[[ -f "$identity" ]] || { echo "Age identity file not found: $identity" >&2; exit 1; }
tmp_dir=$(mktemp -d)
chmod 700 "$tmp_dir"
trap 'rm -rf "$tmp_dir"' EXIT
plain="$tmp_dir/restore.dump"
age --decrypt --identity "$identity" --output "$plain" "$backup"
pg_restore --list "$plain" >/dev/null

# Refuse to restore over existing public-schema objects. Always target a new isolated DB.
object_count=$(psql "$database_url" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 --command "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m','S','f')")
[[ "$object_count" == "0" ]] || { echo "Refusing to restore: target database has $object_count public-schema objects; use a new empty database" >&2; exit 1; }
pg_restore --exit-on-error --no-owner --no-acl --dbname="$database_url" "$plain"
echo "Restored encrypted backup into the previously empty target database."
