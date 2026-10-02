#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
env_file="$root/.env"
[[ -f "$env_file" ]] || { echo 'Run bootstrap.sh first.' >&2; exit 1; }
command -v jq >/dev/null || { echo 'jq is required to configure Keycloak dynamic registration.' >&2; exit 1; }

set -a
# shellcheck disable=SC1090
. "$env_file"
set +a

[[ -n ${KEYCLOAK_DCR_TRUSTED_HOSTS:-} && "$KEYCLOAK_DCR_TRUSTED_HOSTS" != *CHANGE_ME* ]] || {
  echo 'Set KEYCLOAK_DCR_TRUSTED_HOSTS to the approved MCP client hosts.' >&2
  exit 1
}

realm=${OSS_KEYCLOAK_REALM:-voxis-oss}
compose=(docker compose --env-file "$env_file" -f "$root/compose.yaml")
# shellcheck source=lib/keycloak_admin.sh
. "$root/scripts/lib/keycloak_admin.sh"
required_client_scopes=(
  basic
  media:read media:write transcription:read transcription:write
  summary:read summary:write export:read collection:read collection:write
  analysis:read analysis:write email profile
)
policy_scopes=("${required_client_scopes[@]}")

one_policy_id() {
  local components=$1
  local name=$2
  jq -er --arg name "$name" '
    [.[] | select(
      .name == $name and
      .subType == "anonymous" and
      .providerType == "org.keycloak.services.clientregistration.policy.ClientRegistrationPolicy"
    )] | if length == 1 then .[0].id else error("expected exactly one anonymous " + $name + " policy") end
  ' <<<"$components"
}

hosts=()
IFS=',' read -r -a configured_hosts <<<"$KEYCLOAK_DCR_TRUSTED_HOSTS"
for host in "${configured_hosts[@]}"; do
  host=${host//[[:space:]]/}
  [[ "$host" =~ ^[A-Za-z0-9][A-Za-z0-9.-]*$ ]] || {
    echo "Invalid Keycloak DCR trusted host: $host" >&2
    exit 1
  }
  [[ "$host" != *..* && "$host" != .* && "$host" != *. ]] || {
    echo "Invalid Keycloak DCR trusted host: $host" >&2
    exit 1
  }
  hosts+=("$host")
done
(( ${#hosts[@]} > 0 )) || { echo 'No Keycloak DCR trusted hosts were supplied.' >&2; exit 1; }

kc_admin_login
trap kc_admin_logout EXIT

components=$(kc_admin GET "$realm/components")
scope_policy_id=$(one_policy_id "$components" 'Allowed Client Scopes')
host_policy_id=$(one_policy_id "$components" 'Trusted Hosts')
realm_scopes=$(kc_admin GET "$realm/client-scopes")
for scope in "${required_client_scopes[@]}"; do
  jq -e --arg scope "$scope" 'any(.[]; .name == $scope)' <<<"$realm_scopes" >/dev/null || {
    echo "Keycloak realm is missing required client scope: $scope" >&2
    exit 1
  }
done

scopes_json=$(printf '%s\n' "${policy_scopes[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))')
hosts_json=$(printf '%s\n' "${hosts[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))')

# This updates only the existing anonymous policies. The authenticated policy is
# deliberately untouched. Keycloak validates scope names after the realm import.
component=$(kc_admin GET "$realm/components/$scope_policy_id")
payload=$(jq -c --argjson scopes "$scopes_json" '
  .config["allow-default-scopes"] = ["true"] | .config["allowed-client-scopes"] = $scopes
' <<<"$component")
kc_admin PUT "$realm/components/$scope_policy_id" "$payload" >/dev/null
component=$(kc_admin GET "$realm/components/$host_policy_id")
payload=$(jq -c --argjson hosts "$hosts_json" '
  .config["trusted-hosts"] = $hosts
  | .config["host-sending-registration-request-must-match"] = ["true"]
  | .config["client-uris-must-match"] = ["true"]
' <<<"$component")
kc_admin PUT "$realm/components/$host_policy_id" "$payload" >/dev/null

echo 'Configured the anonymous Keycloak DCR policies for the approved hosts and Voxis MCP scopes.'
