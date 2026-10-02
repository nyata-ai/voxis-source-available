#!/usr/bin/env bash
set -euo pipefail

root=${OSS_LLAMA_ATTACK_SURFACE_ROOT:-$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)}
env_file="$root/.env.example"
compose_file="$root/compose.yaml"
runtime_file=$(CDPATH= cd -- "$root/../../editions/oss/runtime/llama" && pwd)/Dockerfile
request_builder="$root/../../backend/internal/adapter/gemma/client.go"
expected_env_sha256='47470b479fb35ed2ac6b3cd6aab856b542f49771f98f23b229698baa7fcd512c'
expected_compose_sha256='f2ae807555069e8da0a090c4729f9e308fa3509752cf08466060842fc3b8f5dd'
expected_runtime_sha256='2ded27b51d165853a07877b3241904b7bc27522662d75f2cd434ffde6053818f'
expected_request_builder_sha256='a197020bf02fee12741035e95e654bd64d8e7139fc3ac07385c74a9adf8d403c'

require_sha256() {
  local file=$1 expected=$2 description=$3 actual
  actual=$(sha256sum "$file" | awk '{print $1}')
  [[ "$actual" == "$expected" ]] || {
    printf 'Llama attack-surface review is stale: %s changed.\n' "$description" >&2
    exit 1
  }
}

require_sha256 "$env_file" "$expected_env_sha256" 'the model environment'
require_sha256 "$compose_file" "$expected_compose_sha256" 'the model Compose service'
require_sha256 "$runtime_file" "$expected_runtime_sha256" 'the Llama runtime build'
require_sha256 "$request_builder" "$expected_request_builder_sha256" 'the Gemma request builder'
