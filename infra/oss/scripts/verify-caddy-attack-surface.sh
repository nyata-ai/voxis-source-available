#!/usr/bin/env bash
set -euo pipefail

root=${OSS_CADDY_ATTACK_SURFACE_ROOT:-$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)}
caddy_file="$root/caddy/Caddyfile"
smoke_file="$root/caddy/Caddyfile.smoke"
web_dockerfile="$root/../../editions/oss/runtime/web/Dockerfile"
caddy_dockerfile="$root/../../editions/oss/runtime/caddy/Dockerfile"
expected_caddy_sha256='12f5e85785161f2f7d88c04efd69659b0c52e6e1ba2b91f57ff8ab82b039bc3b'
expected_smoke_sha256='a776182942319265f02e43cc388ecd81e5860e2bbc6cfb6938bbee9e3c6853c0'
expected_web_sha256='c7235e7252f45d514ab485231f1364e70bc851ee431485be1a13add4db190ee1'
expected_runtime_sha256='ba2bab5bfeafd828bf628eed702a0ea22d5f901bebdce7aa80c47e3c27c33c93'

# Any mention fails, quoted or not, because the web image writes its Caddyfile
# from quoted printf arguments in its Dockerfile.
if grep -nE '(^|[^[:alnum:]_-])(acme_server|pki)($|[^[:alnum:]_-])' "$caddy_file" "$smoke_file" "$web_dockerfile"; then
  printf 'A Caddy configuration mentions the ACME server or PKI app. Review the go-jose statement.\n' >&2
  exit 1
fi

require_sha256() {
  local file=$1 expected=$2 description=$3 actual
  actual=$(sha256sum "$file" | awk '{print $1}')
  [[ "$actual" == "$expected" ]] || {
    printf 'Caddy attack-surface review is stale: %s changed.\n' "$description" >&2
    exit 1
  }
}

require_sha256 "$caddy_file" "$expected_caddy_sha256" 'the public proxy configuration'
require_sha256 "$smoke_file" "$expected_smoke_sha256" 'the smoke proxy configuration'
require_sha256 "$web_dockerfile" "$expected_web_sha256" 'the web image configuration'
require_sha256 "$caddy_dockerfile" "$expected_runtime_sha256" 'the Caddy runtime build'
