#!/usr/bin/env bash
set -euo pipefail

[[ ${VOXIS_OSS_SMOKE:-} == 1 ]] || {
  echo 'This OpenBao initializer is for the disposable OSS smoke environment only.' >&2
  exit 1
}

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
env_file="$root/.env"
[[ -f "$env_file" ]] || { echo 'Run bootstrap.sh first.' >&2; exit 1; }
set -a
# shellcheck disable=SC1090
. "$env_file"
set +a
[[ "$COMPOSE_PROJECT_NAME" == voxis-oss-ci ]] || {
  echo 'The smoke initializer requires COMPOSE_PROJECT_NAME=voxis-oss-ci.' >&2
  exit 1
}
command -v jq >/dev/null || { echo 'jq is required.' >&2; exit 1; }
compose=(docker compose --env-file "$env_file" -f "$root/compose.yaml")
# shellcheck source=lib/openbao.sh
. "$root/scripts/lib/openbao.sh"
case "$OPENBAO_BACKUP_APPROLE_FILE" in
  /*) backup_approle_file="$OPENBAO_BACKUP_APPROLE_FILE" ;;
  *) backup_approle_file="$root/$OPENBAO_BACKUP_APPROLE_FILE" ;;
esac

wait_for_openbao() {
  local pattern=$1
  for _ in $(seq 1 45); do
    openbao_status 2>/dev/null | grep -Eq "$pattern" && return 0
    sleep 1
  done
  return 1
}

wait_for_openbao '"initialized"[[:space:]]*:[[:space:]]*false' || { echo 'OpenBao did not become a fresh store.' >&2; exit 1; }

root_token=''
revoke_root() {
  local status=$?
  trap - EXIT
  if [[ -n "$root_token" ]] && ! openbao_revoke "$root_token"; then
    status=1
  fi
  unset root_token unseal_shares
  exit "$status"
}
trap revoke_root EXIT

init=$("${compose[@]}" exec -T openbao bao operator init -key-shares=1 -key-threshold=1 -format=json)
root_token=$(jq -er '.root_token' <<<"$init")
mapfile -t unseal_shares < <(jq -r '.unseal_keys_b64[]' <<<"$init")
unset init
openbao_unseal
openbao_configure_voxis "$root_token"
app_credentials=$(openbao_issue_secret_id "$root_token" voxis-transit)
backup_credentials=$(openbao_issue_secret_id "$root_token" voxis-backup)

"${compose[@]}" restart openbao >/dev/null
wait_for_openbao '"sealed"[[:space:]]*:[[:space:]]*true' || { echo 'OpenBao did not restart as a sealed initialized store.' >&2; exit 1; }
openbao_unseal
bao_as "$root_token" read -field=role_id auth/approle/role/voxis-transit/role-id >/dev/null
openbao_write_app_approle "$env_file" "$app_credentials"
openbao_write_backup_approle "$backup_approle_file" "$backup_credentials"
unset app_credentials backup_credentials
echo 'Initialized, restarted, and unsealed disposable OpenBao with the restricted AppRoles; the root token is revoked on exit.'
