#!/usr/bin/env bash
set -euo pipefail

# usage: rotate-approle-secret.sh [UNSEAL_SHARES_FILE]
# Issues a new application AppRole secret ID, reloads the API with it, and
# destroys the previous secret IDs only after the API is ready again. The
# custodians supply the unseal-share threshold for a temporary root token.
shares_file=${1:-}
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
env_file="$root/.env"
[[ -f "$env_file" ]] || { echo 'Run bootstrap.sh and init-openbao.sh first.' >&2; exit 1; }
command -v jq >/dev/null || { echo 'jq is required.' >&2; exit 1; }
set -a
# shellcheck disable=SC1090
. "$env_file"
set +a
compose=(docker compose --env-file "$env_file" -f "$root/compose.yaml")
if [[ -n ${VOXIS_OSS_COMPOSE_OVERRIDE_FILES:-} ]]; then
  IFS=: read -r -a compose_overrides <<< "$VOXIS_OSS_COMPOSE_OVERRIDE_FILES"
  for compose_override in "${compose_overrides[@]}"; do
    [[ -f "$compose_override" ]] || { echo "Compose override is missing: $compose_override" >&2; exit 1; }
    compose+=(-f "$compose_override")
  done
fi
# shellcheck source=lib/openbao.sh
. "$root/scripts/lib/openbao.sh"

status=$(openbao_status)
jq -e '.sealed == false' <<<"$status" >/dev/null || { echo 'Unseal OpenBao before rotating the AppRole secret ID.' >&2; exit 1; }
threshold=$(jq -er '.t' <<<"$status")

root_token=''
revoke_root() {
  local exit_status=$?
  trap - EXIT
  if [[ -n "$root_token" ]] && ! openbao_revoke "$root_token"; then
    echo 'Failed to revoke the temporary root token.' >&2
    exit_status=1
  fi
  unset root_token unseal_shares credentials
  exit "$exit_status"
}
trap revoke_root EXIT

openbao_read_shares "$threshold" "$shares_file"
root_token=$(openbao_generate_root)
unset unseal_shares
credentials=$(openbao_issue_secret_id "$root_token" voxis-transit)
openbao_write_app_approle "$env_file" "$credentials"
# Compose prefers exported process values over the file; drop the old ones.
unset VAULT_ROLE_ID VAULT_SECRET_ID
"${compose[@]}" --profile app up -d --no-deps --force-recreate api >/dev/null

ready=0
for _ in $(seq 1 60); do
  if "${compose[@]}" --profile app exec -T api curl --fail --silent http://127.0.0.1:8080/ready >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 2
done
[[ $ready -eq 1 ]] || {
  echo 'The API did not become ready with the new secret ID. The previous secret IDs remain valid; inspect the API before retrying.' >&2
  exit 1
}
openbao_destroy_other_secret_ids "$root_token" voxis-transit "$(jq -r '.accessor' <<<"$credentials")"
echo 'Rotated the application AppRole secret ID, reloaded the API, and destroyed the previous secret IDs.'
