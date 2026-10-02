#!/usr/bin/env bash
set -euo pipefail

root=$(git -C "$(dirname -- "$0")/../.." rev-parse --show-toplevel)
guard="$root/infra/oss/scripts/verify-llama-attack-surface.sh"
fixture=$(mktemp -d)

cleanup() {
  rm -rf -- "$fixture"
}
trap cleanup EXIT

mkdir -p "$fixture/infra/oss" "$fixture/editions/oss/runtime/llama" "$fixture/backend/internal/adapter/gemma"
cp "$root/infra/oss/.env.example" "$fixture/infra/oss/.env.example"
cp "$root/infra/oss/compose.yaml" "$fixture/infra/oss/compose.yaml"
cp "$root/editions/oss/runtime/llama/Dockerfile" "$fixture/editions/oss/runtime/llama/Dockerfile"
cp "$root/backend/internal/adapter/gemma/client.go" "$fixture/backend/internal/adapter/gemma/client.go"

fixture_root="$fixture/infra/oss"
OSS_LLAMA_ATTACK_SURFACE_ROOT="$fixture_root" bash "$guard"
for changed_file in infra/oss/.env.example infra/oss/compose.yaml editions/oss/runtime/llama/Dockerfile backend/internal/adapter/gemma/client.go; do
  printf '\n// simulated Llama attack-surface change\n' >> "$fixture/$changed_file"
  if OSS_LLAMA_ATTACK_SURFACE_ROOT="$fixture_root" bash "$guard" >/dev/null 2>&1; then
    printf 'Llama attack-surface guard accepted changed %s.\n' "$changed_file" >&2
    exit 1
  fi
  cp "$root/$changed_file" "$fixture/$changed_file"
done
