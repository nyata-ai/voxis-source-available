#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
env_file="$root/.env"
[[ -f "$env_file" ]] || { echo 'Run bootstrap.sh first.' >&2; exit 1; }
command -v curl >/dev/null || { echo 'curl is required for the Keycloak DCR smoke check.' >&2; exit 1; }
command -v jq >/dev/null || { echo 'jq is required for the Keycloak DCR smoke check.' >&2; exit 1; }

set -a
# shellcheck disable=SC1090
. "$env_file"
set +a

realm=${OSS_KEYCLOAK_REALM:-voxis-oss}
base_url=${PUBLIC_BASE_URL%/}
endpoint_path="/auth/realms/$realm/clients-registrations/openid-connect"
dcr_url=${KEYCLOAK_DCR_TEST_URL:-"$base_url$endpoint_path"}
redirect_uri=${KEYCLOAK_DCR_TEST_REDIRECT_URI:-"$base_url/mcp/callback"}
case "$dcr_url" in
  *"$endpoint_path") dcr_origin=${dcr_url%"$endpoint_path"} ;;
  *) echo 'Keycloak DCR test URL has an unexpected endpoint path.' >&2; exit 1 ;;
esac
tmpdir=$(mktemp -d)
chmod 700 "$tmpdir"
cleanup_targets=()

cleanup() {
  local target management_path registration_token status=0
  for target in "${cleanup_targets[@]}"; do
    management_path=${target%%$'\t'*}
    registration_token=${target#*$'\t'}
    if ! curl --fail --silent --show-error --connect-timeout 5 --max-time 15 \
      --request DELETE --header "Authorization: Bearer $registration_token" \
      "$dcr_origin$endpoint_path$management_path" >/dev/null; then
      status=1
    fi
  done
  rm -rf "$tmpdir"
  return "$status"
}
trap 'exit_code=$?; cleanup || exit_code=1; exit "$exit_code"' EXIT

track_registration() {
  local response=$1 management_url management_path registration_token
  management_url=$(jq -er '.registration_client_uri' "$response")
  registration_token=$(jq -er '.registration_access_token' "$response")
  case "$management_url" in
    *"$endpoint_path"/*) management_path=${management_url#*"$endpoint_path"} ;;
    *) echo 'Keycloak DCR response has an unexpected management URL.' >&2; return 1 ;;
  esac
  cleanup_targets+=("$management_path"$'\t'"$registration_token")
}

request() {
  local scope=$1
  local expected_status=$2
  local response=$3
  local client_name="voxis-oss-dcr-probe-$(date +%s)-$$-$RANDOM"
  local payload
  if [[ -n "$scope" ]]; then
    payload=$(jq -cn --arg name "$client_name" --arg redirect "$redirect_uri" --arg scope "$scope" \
      '{client_name:$name,redirect_uris:[$redirect],grant_types:["authorization_code","refresh_token"],response_types:["code"],token_endpoint_auth_method:"none",scope:$scope}')
  else
    payload=$(jq -cn --arg name "$client_name" --arg redirect "$redirect_uri" \
      '{client_name:$name,redirect_uris:[$redirect],grant_types:["authorization_code","refresh_token"],response_types:["code"],token_endpoint_auth_method:"none"}')
  fi
  local status
  status=$(curl --silent --show-error --connect-timeout 5 --max-time 15 \
    --output "$response" --write-out '%{http_code}' \
    --header 'Content-Type: application/json' --data-binary "$payload" "$dcr_url")
  [[ "$status" == "$expected_status" ]] || {
    echo "Keycloak DCR probe returned HTTP $status, expected $expected_status." >&2
    return 1
  }
}

allowed="$tmpdir/allowed.json"
omitted="$tmpdir/omitted.json"
rejected="$tmpdir/rejected.json"
request 'media:read' 201 "$allowed"
track_registration "$allowed"
jq -e '.scope | split(" ") | index("media:read") != null' "$allowed" >/dev/null
request '' 201 "$omitted"
track_registration "$omitted"
! jq -e '.scope | split(" ") | index("media:read") != null' "$omitted" >/dev/null
request 'not-a-voxis-scope' 403 "$rejected"
echo 'Anonymous Keycloak dynamic registration accepted approved scopes, omitted unrequested scopes, and rejected an unknown scope.'
