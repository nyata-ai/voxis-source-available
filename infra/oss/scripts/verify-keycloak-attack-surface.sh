#!/usr/bin/env bash
set -euo pipefail

root=${OSS_KEYCLOAK_ATTACK_SURFACE_ROOT:-$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)}
compose_file="$root/compose.yaml"
realm_file="$root/keycloak/realm-template.json"
caddy_file="$root/caddy/Caddyfile"
env_file="$root/.env.example"
runtime_dockerfile=${OSS_KEYCLOAK_RUNTIME_DOCKERFILE:-"$root/../../editions/oss/runtime/keycloak/Dockerfile"}
expected_image='quay.io/keycloak/keycloak@sha256:6529db34610ce6328824218dce3bbe55c4ba4757bc5c8a09b5a4c746e28b6e9d'
expected_temurin_image='eclipse-temurin@sha256:db724162e4ff5f71fa0a2815026314d9e5e0e9a139f49be1d4717e6930e93341'
expected_compose_sha256='898e80c60a3177c7ffaf0c761691c0c82bcd7492ea334afe6bb24c0decbd35d4'
expected_realm_sha256='c287ba158d3f807d3a2636d18fb04be723638a587846fb6e3699279159b3f88b'
expected_caddy_sha256='12f5e85785161f2f7d88c04efd69659b0c52e6e1ba2b91f57ff8ab82b039bc3b'
expected_runtime_dockerfile_sha256='cf45d9fe7c2e2f799c791b728e9160dba9ad639e113f62759a06f1cc0c1b022f'

require_sha256() {
  local file=$1 expected=$2 description=$3 actual
  actual=$(sha256sum "$file" | awk '{print $1}')
  [[ "$actual" == "$expected" ]] || {
    printf 'Keycloak attack-surface review is stale: %s changed.\n' "$description" >&2
    exit 1
  }
}

keycloak_image=$(awk -F= '$1 == "KEYCLOAK_IMAGE" { print $2; exit }' "$env_file")
[[ "$keycloak_image" == "$expected_image" ]] || {
  printf 'The reviewed Keycloak attack surface applies only to the pinned 26.7.5 source image.\n' >&2
  exit 1
}

temurin_image=$(awk -F= '$1 == "TEMURIN_IMAGE" { print $2; exit }' "$env_file")
[[ "$temurin_image" == "$expected_temurin_image" ]] || {
  printf 'The reviewed Keycloak attack surface applies only to the pinned Temurin 21 JRE image.\n' >&2
  exit 1
}

require_sha256 "$compose_file" "$expected_compose_sha256" 'the Keycloak Compose service'
require_sha256 "$realm_file" "$expected_realm_sha256" 'the imported Keycloak realm'
require_sha256 "$caddy_file" "$expected_caddy_sha256" 'the public Keycloak proxy'
require_sha256 "$runtime_dockerfile" "$expected_runtime_dockerfile_sha256" 'the Keycloak runtime Dockerfile'
