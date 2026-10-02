#!/usr/bin/env bash
set -euo pipefail

root=$(git -C "$(dirname -- "$0")/../.." rev-parse --show-toplevel)
fixture=$(mktemp -d)

cleanup() {
  rm -rf -- "$fixture"
}
trap cleanup EXIT

git -C "$fixture" init --quiet
cp "$root/.gitignore" "$fixture/.gitignore"

paths=(
  "infra/oss/secrets/openbao/operator-init.json"
  "infra/oss/secrets/openbao/backup-approle.json"
  "infra/oss/.oss-ci/recovery/operator-init.json"
  "infra/oss/.oss-ci/recovery/backup-approle.json"
  "infra/oss/keycloak/realm-voxis-oss.json"
  "infra/oss/.env"
)

for relative in "${paths[@]}"; do
  path="$fixture/$relative"
  mkdir -p "$(dirname -- "$path")"
  printf '{"synthetic":true}\n' > "$path"
  git -C "$fixture" check-ignore -q -- "$relative" || {
    printf 'Generated installation material is not ignored: %s\n' "$relative" >&2
    exit 1
  }
  if git -C "$fixture" add --dry-run -- "$relative" >/dev/null 2>&1; then
    printf 'Generated installation material would be staged: %s\n' "$relative" >&2
    exit 1
  fi
done
