#!/usr/bin/env bash
set -euo pipefail

root=$(git -C "$(dirname -- "$0")/../.." rev-parse --show-toplevel)
guard="$root/infra/oss/scripts/verify-caddy-attack-surface.sh"
fixture=$(mktemp -d)

cleanup() {
  rm -rf -- "$fixture"
}
trap cleanup EXIT

files=(infra/oss/caddy/Caddyfile infra/oss/caddy/Caddyfile.smoke editions/oss/runtime/web/Dockerfile editions/oss/runtime/caddy/Dockerfile)
for file in "${files[@]}"; do
  mkdir -p "$fixture/$(dirname -- "$file")"
  cp "$root/$file" "$fixture/$file"
done

fixture_root="$fixture/infra/oss"
OSS_CADDY_ATTACK_SURFACE_ROOT="$fixture_root" bash "$guard"
for changed_file in "${files[@]}"; do
  printf '\n# simulated Caddy attack-surface change\n' >> "$fixture/$changed_file"
  if OSS_CADDY_ATTACK_SURFACE_ROOT="$fixture_root" bash "$guard" >/dev/null 2>&1; then
    printf 'Caddy attack-surface guard accepted changed %s.\n' "$changed_file" >&2
    exit 1
  fi
  cp "$root/$changed_file" "$fixture/$changed_file"
done

# Each form must trip the directive check itself, not only the digest behind it.
for line in "  acme_server" "  acme_server {" "    '  acme_server' \\" "    '  acme_server {' \\" "	pki {" "    '  pki {' \\"; do
  for target in infra/oss/caddy/Caddyfile editions/oss/runtime/web/Dockerfile; do
    printf '%s\n' "$line" >> "$fixture/$target"
    if output=$(OSS_CADDY_ATTACK_SURFACE_ROOT="$fixture_root" bash "$guard" 2>&1); then
      printf 'Caddy attack-surface guard accepted %s in %s.\n' "$line" "$target" >&2
      exit 1
    fi
    [[ "$output" == *'ACME server or PKI app'* ]] || {
      printf 'The directive check did not catch %s in %s.\n' "$line" "$target" >&2
      exit 1
    }
    cp "$root/$target" "$fixture/$target"
  done
done
