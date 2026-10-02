#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
lock_file="$root/images.lock.yaml"
ci_env_file="$root/../../editions/oss/.env.ci"

lock_value() {
  local image=$1 field=$2
  awk -v image="$image" -v field="$field" '
    $0 == "  " image ":" { found = 1; next }
    found && /^  [^ ]/ { exit }
    found && $0 ~ "^    " field ": " {
      sub("^    " field ": ", "")
      print
      exit
    }
  ' "$lock_file"
}

env_value() {
  local file=$1 variable=$2
  awk -v variable="$variable" '
    index($0, variable "=") == 1 {
      print substr($0, length(variable) + 2)
      exit
    }
  ' "$file"
}

require_digest() {
  local value=$1 label=$2
  [[ "$value" =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || {
    printf '%s must be an immutable SHA-256 image reference.\n' "$label" >&2
    exit 1
  }
}

require_equal() {
  local actual=$1 expected=$2 label=$3
  [[ -n "$actual" && "$actual" == "$expected" ]] || {
    printf '%s does not match the image lock.\n' "$label" >&2
    exit 1
  }
}

for mapping in \
  'postgres POSTGRES_IMAGE' \
  'keycloak KEYCLOAK_IMAGE' \
  'temurin_21_jre TEMURIN_IMAGE' \
  'openbao OPENBAO_IMAGE' \
  'clamav CLAMAV_IMAGE'; do
  read -r image variable <<< "$mapping"
  reference=$(lock_value "$image" reference)
  require_digest "$reference" "images.lock.yaml $image reference"
  require_equal "$(env_value "$root/.env.example" "$variable")" "$reference" ".env.example $variable"
  require_equal "$(env_value "$ci_env_file" "$variable")" "$reference" "editions/oss/.env.ci $variable"
done

ci_workflow="$root/../../.github/workflows/ci.yml"
postgres_reference=$(lock_value postgres reference)
grep -Fxq "        image: $postgres_reference" "$ci_workflow" || {
  printf 'The CI PostgreSQL service image does not match the image lock.\n' >&2
  exit 1
}

keycloak_tag=$(lock_value keycloak source_tag)
keycloak_version=${keycloak_tag#quay.io/keycloak/keycloak:}
minimum_keycloak_version=26.7.5
[[ "$keycloak_tag" == "quay.io/keycloak/keycloak:$keycloak_version" ]] || {
  printf 'Keycloak source tag is malformed.\n' >&2
  exit 1
}
[[ "$(printf '%s\n%s\n' "$minimum_keycloak_version" "$keycloak_version" | sort -V | head -n1)" == "$minimum_keycloak_version" ]] || {
  printf 'Keycloak must be at least %s.\n' "$minimum_keycloak_version" >&2
  exit 1
}
