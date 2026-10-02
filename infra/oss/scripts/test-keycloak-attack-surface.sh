#!/usr/bin/env bash
set -euo pipefail

root=$(git -C "$(dirname -- "$0")/../.." rev-parse --show-toplevel)
guard="$root/infra/oss/scripts/verify-keycloak-attack-surface.sh"
fixture=$(mktemp -d)

cleanup() {
  rm -rf -- "$fixture"
}
trap cleanup EXIT

mkdir -p "$fixture/keycloak" "$fixture/caddy" "$fixture/runtime"
cp "$root/infra/oss/compose.yaml" "$fixture/compose.yaml"
cp "$root/infra/oss/keycloak/realm-template.json" "$fixture/keycloak/realm-template.json"
cp "$root/infra/oss/caddy/Caddyfile" "$fixture/caddy/Caddyfile"
cp "$root/infra/oss/.env.example" "$fixture/.env.example"
cp "$root/editions/oss/runtime/keycloak/Dockerfile" "$fixture/runtime/Dockerfile"

OSS_KEYCLOAK_ATTACK_SURFACE_ROOT="$fixture" \
  OSS_KEYCLOAK_RUNTIME_DOCKERFILE="$fixture/runtime/Dockerfile" \
  bash "$guard"
printf '\n# simulated Keycloak attack-surface change\n' >> "$fixture/compose.yaml"
if OSS_KEYCLOAK_ATTACK_SURFACE_ROOT="$fixture" \
  OSS_KEYCLOAK_RUNTIME_DOCKERFILE="$fixture/runtime/Dockerfile" \
  bash "$guard" >/dev/null 2>&1; then
  printf 'Keycloak attack-surface guard accepted a changed Compose file.\n' >&2
  exit 1
fi

cp "$root/infra/oss/compose.yaml" "$fixture/compose.yaml"
printf '\n# simulated Keycloak runtime change\n' >> "$fixture/runtime/Dockerfile"
if OSS_KEYCLOAK_ATTACK_SURFACE_ROOT="$fixture" \
  OSS_KEYCLOAK_RUNTIME_DOCKERFILE="$fixture/runtime/Dockerfile" \
  bash "$guard" >/dev/null 2>&1; then
  printf 'Keycloak attack-surface guard accepted a changed runtime Dockerfile.\n' >&2
  exit 1
fi
