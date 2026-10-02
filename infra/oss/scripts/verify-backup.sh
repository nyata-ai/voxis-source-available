#!/usr/bin/env bash
set -euo pipefail

backup_dir=${1:?usage: verify-backup.sh BACKUP_DIR}
[[ -n ${BACKUP_AGE_IDENTITY:-} ]] || { echo 'Set BACKUP_AGE_IDENTITY to the age identity file.' >&2; exit 1; }
command -v age >/dev/null || { echo 'age is required.' >&2; exit 1; }
(
  cd "$backup_dir"
  sha256sum --check SHA256SUMS
)
for file in "$backup_dir"/*.age; do
  age --decrypt -i "$BACKUP_AGE_IDENTITY" "$file" >/dev/null
done
printf 'Backup hashes and decryption streams verified. This does not modify a restore target.\n'
