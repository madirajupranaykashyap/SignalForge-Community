#!/usr/bin/env bash
set -euo pipefail

usage() { echo "Usage: $0 DATABASE_URL AGE_RECIPIENT OUTPUT_DIRECTORY" >&2; exit 2; }
[[ $# -eq 3 ]] || usage
database_url=$1
recipient=$2
out_dir=$3
for tool in pg_dump pg_restore age; do
  command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
done
[[ "$database_url" == postgres://* || "$database_url" == postgresql://* ]] || { echo "DATABASE_URL must use postgres:// or postgresql://" >&2; exit 2; }
[[ "$recipient" == age1* ]] || { echo "AGE_RECIPIENT must be an age public recipient (age1...)" >&2; exit 2; }
mkdir -p "$out_dir"
chmod 700 "$out_dir"
tmp_dir=$(mktemp -d "$out_dir/.signalforge-pg-backup.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT
stamp=$(date -u +%Y%m%dT%H%M%SZ)
plain="$tmp_dir/management-$stamp.dump"
encrypted_tmp="$tmp_dir/management-$stamp.dump.age"
dest="$out_dir/management-$stamp.dump.age"
[[ ! -e "$dest" ]] || { echo "Backup already exists: $dest" >&2; exit 1; }

# The plaintext dump exists only in the private temporary directory and is deleted on exit.
pg_dump --format=custom --no-owner --no-acl --dbname="$database_url" --file="$plain"
pg_restore --list "$plain" >/dev/null
age --encrypt --recipient "$recipient" --output "$encrypted_tmp" "$plain"
[[ -s "$encrypted_tmp" ]] || { echo "Encrypted backup is empty" >&2; exit 1; }
chmod 600 "$encrypted_tmp"
mv "$encrypted_tmp" "$dest"
echo "$dest"
