#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
env_file="$root/.env"
[[ -f "$env_file" ]] || { echo 'Run bootstrap.sh first.' >&2; exit 1; }
command -v jq >/dev/null || { echo 'jq is required.' >&2; exit 1; }
set -a
# shellcheck disable=SC1090
. "$env_file"
set +a
compose=(docker compose --env-file "$env_file" -f "$root/compose.yaml")
# shellcheck source=lib/openbao.sh
. "$root/scripts/lib/openbao.sh"
resolve_path() {
  case "$1" in
    /*) printf '%s\n' "$1" ;;
    *) printf '%s/%s\n' "$root" "$1" ;;
  esac
}
recovery_dir=$(resolve_path "$OPENBAO_RECOVERY_DIR")
backup_approle_file=$(resolve_path "$OPENBAO_BACKUP_APPROLE_FILE")
init_file="$recovery_dir/operator-init.json"

[[ ! -e "$init_file" ]] || { echo 'OpenBao recovery material already exists; refusing to initialize a replacement store.' >&2; exit 1; }
openbao_status | grep -Eq '"initialized"[[:space:]]*:[[:space:]]*false' || { echo 'OpenBao is already initialized; refusing to replace it.' >&2; exit 1; }

root_token=''
revoke_root() {
  local status=$?
  trap - EXIT
  if [[ -n "$root_token" ]] && ! openbao_revoke "$root_token"; then
    echo 'Failed to revoke the initial root token.' >&2
    status=1
  fi
  unset root_token unseal_shares
  exit "$status"
}
trap revoke_root EXIT

umask 077
mkdir -p "$recovery_dir"
init=$("${compose[@]}" exec -T openbao bao operator init -key-shares=5 -key-threshold=3 -format=json)
root_token=$(jq -er '.root_token' <<<"$init")
# Only the unseal shares are written. The root token stays in memory and is
# revoked when this script exits, even after a failure.
jq '{unseal_keys_b64, unseal_shares, unseal_threshold}' <<<"$init" > "$init_file"
chmod 600 "$init_file"
mapfile -t unseal_shares < <(jq -r '.unseal_keys_b64[0:3][]' <<<"$init")
unset init
openbao_unseal

openbao_configure_voxis "$root_token"
app_credentials=$(openbao_issue_secret_id "$root_token" voxis-transit)
openbao_write_app_approle "$env_file" "$app_credentials"
backup_credentials=$(openbao_issue_secret_id "$root_token" voxis-backup)
openbao_write_backup_approle "$backup_approle_file" "$backup_credentials"
unset app_credentials backup_credentials

cat <<EOF
OpenBao is initialized and unsealed. The initial root token is revoked when this
script exits; nothing on this host can act as root.

The five unseal shares are in:
  $init_file
Any three of them unseal OpenBao and can generate a temporary root token for
recovery. Give each share to a different custodian now, store them offline, and
then delete the file from this host:
  shred -u '$init_file'

The application AppRole is in .env. The backup AppRole is in
  $backup_approle_file
and only works from inside the OpenBao container.
EOF
