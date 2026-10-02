#!/usr/bin/env bash
set -euo pipefail

root=${OSS_OPENBAO_ATTACK_SURFACE_ROOT:-$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)}
env_file="$root/.env.example"
compose_file="$root/compose.yaml"
config_file="$root/openbao/openbao.hcl"
init_file="$root/scripts/init-openbao.sh"
lib_file="$root/scripts/lib/openbao.sh"
runtime_file=$(CDPATH= cd -- "$root/../../editions/oss/runtime/openbao" && pwd)/Dockerfile
expected_image='quay.io/openbao/openbao@sha256:a60afafda36337abe833c4a63894bf1095098f29abea4091e7e555a33dd52889'
expected_env_sha256='47470b479fb35ed2ac6b3cd6aab856b542f49771f98f23b229698baa7fcd512c'
expected_compose_sha256='f2ae807555069e8da0a090c4729f9e308fa3509752cf08466060842fc3b8f5dd'
expected_config_sha256='e6884ea2f45381e775eb15f1e33bacc097b0c15976ada30e52d36bbaf00af536'
expected_init_sha256='f8ffba4c1c2be71d4d2a80f954f11ad90769de24ee71d193f73e428de3303feb'
expected_lib_sha256='b330052fcff83b9babd0656f53404ec0a1a391fc8b4ffa68f5c11ff5e237c7e3'
expected_runtime_sha256='30ab6d5a669783c1ab0b3023db4fdf7bcd11348f82335c33545984e901e78823'

require_sha256() {
  local file=$1 expected=$2 description=$3 actual
  actual=$(sha256sum "$file" | awk '{print $1}')
  [[ "$actual" == "$expected" ]] || {
    printf 'OpenBao attack-surface review is stale: %s changed.\n' "$description" >&2
    exit 1
  }
}

openbao_image=$(awk -F= '$1 == "OPENBAO_IMAGE" { print $2; exit }' "$env_file")
[[ "$openbao_image" == "$expected_image" ]] || {
  printf 'The reviewed OpenBao attack surface applies only to the pinned 2.6.3 image.\n' >&2
  exit 1
}

require_sha256 "$env_file" "$expected_env_sha256" 'the locked OpenBao image'
require_sha256 "$compose_file" "$expected_compose_sha256" 'the OpenBao Compose service and network'
require_sha256 "$config_file" "$expected_config_sha256" 'the OpenBao listener configuration'
require_sha256 "$init_file" "$expected_init_sha256" 'the OpenBao initialization script'
require_sha256 "$lib_file" "$expected_lib_sha256" 'the OpenBao policies and AppRoles'
require_sha256 "$runtime_file" "$expected_runtime_sha256" 'the OpenBao runtime image build'
