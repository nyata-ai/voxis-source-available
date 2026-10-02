#!/usr/bin/env bash
set -euo pipefail

root=$(git -C "$(dirname -- "$0")/../.." rev-parse --show-toplevel)
guard="$root/infra/oss/scripts/verify-openbao-attack-surface.sh"
fixture=$(mktemp -d)

cleanup() {
  rm -rf -- "$fixture"
}
trap cleanup EXIT

mkdir -p "$fixture/infra/oss/openbao" "$fixture/infra/oss/scripts/lib" "$fixture/editions/oss/runtime/openbao"
cp "$root/infra/oss/.env.example" "$fixture/infra/oss/.env.example"
cp "$root/infra/oss/compose.yaml" "$fixture/infra/oss/compose.yaml"
cp "$root/infra/oss/openbao/openbao.hcl" "$fixture/infra/oss/openbao/openbao.hcl"
cp "$root/infra/oss/scripts/init-openbao.sh" "$fixture/infra/oss/scripts/init-openbao.sh"
cp "$root/infra/oss/scripts/lib/openbao.sh" "$fixture/infra/oss/scripts/lib/openbao.sh"
cp "$root/editions/oss/runtime/openbao/Dockerfile" "$fixture/editions/oss/runtime/openbao/Dockerfile"

fixture_root="$fixture/infra/oss"
OSS_OPENBAO_ATTACK_SURFACE_ROOT="$fixture_root" bash "$guard"
for changed_file in infra/oss/.env.example infra/oss/compose.yaml infra/oss/openbao/openbao.hcl infra/oss/scripts/init-openbao.sh infra/oss/scripts/lib/openbao.sh editions/oss/runtime/openbao/Dockerfile; do
  printf '\n# simulated OpenBao attack-surface change\n' >> "$fixture/$changed_file"
  if OSS_OPENBAO_ATTACK_SURFACE_ROOT="$fixture_root" bash "$guard" >/dev/null 2>&1; then
    printf 'OpenBao attack-surface guard accepted changed %s.\n' "$changed_file" >&2
    exit 1
  fi
  cp "$root/$changed_file" "$fixture/$changed_file"
done
