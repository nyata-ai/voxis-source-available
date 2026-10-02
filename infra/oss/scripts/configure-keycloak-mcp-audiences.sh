#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
env_file="$root/.env"
[[ -f "$env_file" ]] || { echo 'Run bootstrap.sh first.' >&2; exit 1; }
command -v jq >/dev/null || { echo 'jq is required to update Keycloak MCP audiences.' >&2; exit 1; }

set -a
# shellcheck disable=SC1090
. "$env_file"
set +a

realm=${OSS_KEYCLOAK_REALM:-voxis-oss}
public_base_url=${PUBLIC_BASE_URL:-}
public_base_url=${public_base_url%/}
[[ -n "$public_base_url" ]] || {
  echo 'PUBLIC_BASE_URL is required to update Keycloak MCP audiences.' >&2
  exit 1
}
resource_audience=${MCP_RESOURCE_AUDIENCE:-}
[[ "$resource_audience" == "$public_base_url/mcp" ]] || {
  echo 'MCP_RESOURCE_AUDIENCE must equal PUBLIC_BASE_URL plus /mcp.' >&2
  exit 1
}

compose=(docker compose --env-file "$env_file" -f "$root/compose.yaml")
# shellcheck source=lib/keycloak_admin.sh
. "$root/scripts/lib/keycloak_admin.sh"
scopes=(
  media:read media:write transcription:read transcription:write
  summary:read summary:write export:read collection:read collection:write
  analysis:read analysis:write
)

scope_id() {
  local all_scopes=$1
  local scope=$2
  jq -er --arg scope "$scope" '
    [.[] | select(.name == $scope)] |
    if length == 1 then .[0].id else error("expected exactly one client scope named " + $scope) end
  ' <<<"$all_scopes"
}

mapper_payload() {
  local name=$1
  local audience_key=$2
  local audience=$3
  jq -cn --arg name "$name" --arg key "$audience_key" --arg audience "$audience" '
    {
      name: $name,
      protocol: "openid-connect",
      protocolMapper: "oidc-audience-mapper",
      config: {
        ($key): $audience,
        "access.token.claim": "true",
        "id.token.claim": "false",
        "introspection.token.claim": "true"
      }
    }
  '
}

remove_legacy_mapper() {
  local scope=$1
  local scope_id=$2
  local mappers=$3
  local legacy_id
  legacy_id=$(jq -r --arg api "voxis-oss-api" --arg resource "$resource_audience" --arg scope "$scope" '
    [
      .[] |
      select(
        .protocol == "openid-connect" and
        .protocolMapper == "oidc-audience-mapper" and
        .config["included.client.audience"] == $api and
        .config["included.custom.audience"] == $resource
      )
    ] |
    if length == 0 then empty
    elif length == 1 and .[0].name == "voxis-mcp-audience" then .[0].id
    else error("unexpected combined audience mapper in " + $scope)
    end
  ' <<<"$mappers")
  [[ -z "$legacy_id" ]] && return
  kc_admin DELETE "$realm/client-scopes/$scope_id/protocol-mappers/models/$legacy_id" >/dev/null
}

upsert_mapper() {
  local scope_id=$1
  local mappers=$2
  local name=$3
  local audience_key=$4
  local audience=$5
  local existing_id payload
  existing_id=$(jq -r --arg name "$name" '
    [.[] | select(.name == $name)] |
    if length == 0 then empty
    elif length == 1 and .[0].protocol == "openid-connect" and .[0].protocolMapper == "oidc-audience-mapper" then .[0].id
    else error("unexpected mapper named " + $name)
    end
  ' <<<"$mappers")
  payload=$(mapper_payload "$name" "$audience_key" "$audience")
  if [[ -n "$existing_id" ]]; then
    payload=$(jq -c --arg id "$existing_id" '.id = $id' <<<"$payload")
    kc_admin PUT "$realm/client-scopes/$scope_id/protocol-mappers/models/$existing_id" "$payload" >/dev/null
    return
  fi
  kc_admin POST "$realm/client-scopes/$scope_id/protocol-mappers/models" "$payload" >/dev/null
}

verify_scope() {
  local scope=$1
  local scope_id=$2
  local mappers
  mappers=$(kc_admin GET "$realm/client-scopes/$scope_id/protocol-mappers/models")
  jq -e --arg api "voxis-oss-api" --arg resource "$resource_audience" '
    def one_mapper($name; $key; $value):
      [.[] | select(
        .name == $name and
        .protocol == "openid-connect" and
        .protocolMapper == "oidc-audience-mapper" and
        .config[$key] == $value and
        .config["access.token.claim"] == "true" and
        .config["id.token.claim"] == "false" and
        .config["introspection.token.claim"] == "true"
      )] | length == 1;
    one_mapper("voxis-api-audience"; "included.client.audience"; $api) and
    one_mapper("voxis-mcp-resource-audience"; "included.custom.audience"; $resource) and
    ([.[] | select(
      .protocolMapper == "oidc-audience-mapper" and
      .config["included.client.audience"] == $api and
      .config["included.custom.audience"] == $resource
    )] | length == 0)
  ' <<<"$mappers" >/dev/null || {
    echo "Keycloak MCP audience verification failed for client scope: $scope" >&2
    exit 1
  }
}

kc_admin_login
trap kc_admin_logout EXIT

all_scopes=$(kc_admin GET "$realm/client-scopes")
for scope in "${scopes[@]}"; do
  id=$(scope_id "$all_scopes" "$scope")
  mappers=$(kc_admin GET "$realm/client-scopes/$id/protocol-mappers/models")
  remove_legacy_mapper "$scope" "$id" "$mappers"
  upsert_mapper "$id" "$mappers" 'voxis-api-audience' 'included.client.audience' 'voxis-oss-api'
  upsert_mapper "$id" "$mappers" 'voxis-mcp-resource-audience' 'included.custom.audience' "$resource_audience"
  verify_scope "$scope" "$id"
done

echo 'Updated the retained Keycloak MCP scope audience mappers without changing users, roles, clients, or other scopes.'
