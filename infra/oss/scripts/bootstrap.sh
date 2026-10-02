#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
env_file="$root/.env"
umask 077

host_os=$(uname -s)
host_arch=$(uname -m)
[[ "$host_os" == Linux && "$host_arch" == x86_64 ]] || {
  printf 'Voxis Source-Available requires Linux x86_64/amd64; found %s %s.\n' "$host_os" "$host_arch" >&2
  exit 1
}

privileged() {
  if [[ $EUID -eq 0 ]]; then
    "$@"
    return
  fi
  command -v sudo >/dev/null || { echo 'bootstrap.sh requires sudo to prepare API-owned directories.' >&2; return 1; }
  sudo "$@"
}

resolve_path() {
  case "$1" in
    /*) printf '%s\n' "$1" ;;
    *) printf '%s/%s\n' "$root" "$1" ;;
  esac
}

if [[ ! -f "$env_file" ]]; then
  cp "$root/.env.example" "$env_file"
  printf 'Created %s. Set public URLs and the Speechmatics credential, then rerun this script.\n' "$env_file"
  exit 1
fi

set -a
# shellcheck disable=SC1090
. "$env_file"
set +a

for key in PUBLIC_BASE_URL PUBLIC_FRONTEND_URL CORS_ALLOWED_ORIGINS MCP_ALLOWED_ORIGINS PUBLIC_HOSTNAME TLS_EMAIL OSS_KEYCLOAK_PUBLIC_URL OSS_KEYCLOAK_FETCH_URL MCP_RESOURCE_AUDIENCE OSS_KEYCLOAK_ADDITIONAL_AUDIENCES KEYCLOAK_DCR_TRUSTED_HOSTS SPEECHMATICS_API_KEY; do
  value=${!key:-}
  if [[ -z "$value" || "$value" == *CHANGE_ME* || "$value" == *example.invalid* ]]; then
    printf '%s must be set in %s before bootstrap.\n' "$key" "$env_file" >&2
    exit 1
  fi
done

public_base_url=${PUBLIC_BASE_URL%/}
[[ "$PUBLIC_FRONTEND_URL" == "$public_base_url" ]] || { echo 'PUBLIC_FRONTEND_URL must equal PUBLIC_BASE_URL for same-origin web and API requests.' >&2; exit 1; }
[[ "$CORS_ALLOWED_ORIGINS" == "$public_base_url" ]] || { echo 'CORS_ALLOWED_ORIGINS must equal the public application origin.' >&2; exit 1; }
[[ "$MCP_ALLOWED_ORIGINS" == "$public_base_url" ]] || { echo 'MCP_ALLOWED_ORIGINS must equal the public application origin.' >&2; exit 1; }
[[ "$OSS_KEYCLOAK_PUBLIC_URL" == "$public_base_url/auth" ]] || { echo 'OSS_KEYCLOAK_PUBLIC_URL must be the public application issuer at /auth.' >&2; exit 1; }
[[ "$OSS_KEYCLOAK_FETCH_URL" == http://keycloak:8080/auth ]] || { echo 'OSS_KEYCLOAK_FETCH_URL must use the internal Keycloak discovery address.' >&2; exit 1; }
[[ "$MCP_RESOURCE_AUDIENCE" == "$public_base_url/mcp" ]] || { echo 'MCP_RESOURCE_AUDIENCE must equal PUBLIC_BASE_URL plus /mcp.' >&2; exit 1; }
[[ "$OSS_KEYCLOAK_ADDITIONAL_AUDIENCES" == "$MCP_RESOURCE_AUDIENCE" ]] || { echo 'OSS_KEYCLOAK_ADDITIONAL_AUDIENCES must contain the exact MCP resource audience.' >&2; exit 1; }
[[ "${KEYCLOAK_USER_LOOKUP_CLIENT_ID:-}" == voxis-oss-api-lookup ]] || { echo 'KEYCLOAK_USER_LOOKUP_CLIENT_ID must be voxis-oss-api-lookup, the client the realm import creates.' >&2; exit 1; }
[[ "${OSS_KEYCLOAK_ADMIN_PORT:-8081}" =~ ^[0-9]+$ ]] || { echo 'OSS_KEYCLOAK_ADMIN_PORT must be a port number.' >&2; exit 1; }

replace_secret() {
  local key=$1
  local value
  local env_tmp
  value=$(openssl rand -hex 32)
  env_tmp=$(mktemp "${env_file}.tmp.XXXXXX")
  chmod 600 "$env_tmp"
  if ! SECRET_KEY="$key" SECRET_VALUE="$value" awk '
    $0 == ENVIRON["SECRET_KEY"] "=CHANGE_ME" {
      print ENVIRON["SECRET_KEY"] "=" ENVIRON["SECRET_VALUE"]
      next
    }
    { print }
  ' "$env_file" > "$env_tmp"; then
    rm -f "$env_tmp"
    exit 1
  fi
  mv "$env_tmp" "$env_file"
}

ensure_directory() {
  local path=$1
  [[ ! -e "$path" || -d "$path" ]] || { echo "Path is not a directory: $path" >&2; exit 1; }
  [[ -d "$path" ]] && return
  mkdir -p "$path" 2>/dev/null || privileged mkdir -p "$path"
}

ensure_operator_directory() {
  local path=$1 ancestor created_path
  local -a created_paths=()
  [[ ! -e "$path" || -d "$path" ]] || { echo "Path is not a directory: $path" >&2; exit 1; }
  if [[ ! -d "$path" ]]; then
    if ! mkdir -p "$path" 2>/dev/null; then
      ancestor=$path
      while [[ ! -e "$ancestor" ]]; do
        created_paths+=("$ancestor")
        ancestor=$(dirname -- "$ancestor")
      done
      [[ -d "$ancestor" ]] || { echo "Path is not a directory: $ancestor" >&2; exit 1; }
      privileged mkdir -p "$path"
      for created_path in "${created_paths[@]}"; do
        privileged chown "$(id -u):$(id -g)" "$created_path"
        privileged chmod 700 "$created_path"
      done
    fi
  fi
  [[ -w "$path" && -x "$path" ]] || {
    echo "Operator cannot access $path; adjust its owner before bootstrap." >&2
    exit 1
  }
  chmod 700 "$path"
}

ensure_api_directory() {
  local path=$1 entries owner
  ensure_directory "$path"
  entries=$(privileged find "$path" -mindepth 1 -maxdepth 1 -print -quit) || {
    echo "Cannot inspect $path; refusing to change it." >&2
    exit 1
  }
  if [[ -n "$entries" ]]; then
    owner=$(privileged stat -c '%u:%g' "$path") || {
      echo "Cannot read the owner of $path; refusing to change retained data." >&2
      exit 1
    }
    [[ "$owner" == 10001:10001 ]] || {
      echo "Existing $path must be owned by 10001:10001; refusing to change retained data." >&2
      exit 1
    }
    return
  fi
  privileged chown 10001:10001 "$path"
  privileged chmod 700 "$path"
}

ensure_openbao_data_directory() {
  local path=$1 entries owner mode
  ensure_directory "$path"
  entries=$(privileged find "$path" -mindepth 1 -maxdepth 1 -print -quit) || {
    echo "Cannot inspect $path; refusing to change OpenBao data." >&2
    exit 1
  }
  if [[ -z "$entries" ]]; then
    privileged chown 100:1000 "$path"
    privileged chmod 700 "$path"
    return
  fi
  owner=$(privileged stat -c '%u:%g' "$path") || {
    echo "Cannot read the OpenBao data owner; refusing to change retained data." >&2
    exit 1
  }
  mode=$(privileged stat -c '%a' "$path") || {
    echo "Cannot read OpenBao data permissions; refusing to change retained data." >&2
    exit 1
  }
  [[ "$owner" == 100:1000 && "$mode" == 700 ]] || {
    echo "Existing OpenBao data must be owned by 100:1000 with mode 700; refusing to change retained data." >&2
    exit 1
  }
}

for key in POSTGRES_APP_PASSWORD POSTGRES_BOOTSTRAP_PASSWORD KEYCLOAK_DB_PASSWORD KEYCLOAK_DB_BOOTSTRAP_PASSWORD KEYCLOAK_BOOTSTRAP_ADMIN_PASSWORD KEYCLOAK_USER_LOOKUP_CLIENT_SECRET MEDIA_STREAM_TOKEN_SECRET; do
  if grep -q "^${key}=CHANGE_ME$" "$env_file"; then
    replace_secret "$key"
  fi
done
placeholders=$(awk -F= '/^[A-Za-z_][A-Za-z0-9_]*=.*CHANGE_ME/ { print $1 }' "$env_file")
[[ -z "$placeholders" ]] || {
  printf 'Replace the CHANGE_ME value of %s in %s before bootstrap.\n' "$(tr '\n' ' ' <<<"$placeholders")" "$env_file" >&2
  exit 1
}

set -a
# shellcheck disable=SC1090
. "$env_file"
set +a

media_dir=$(resolve_path "$LOCAL_STORAGE_DIR")
scratch_dir=$(resolve_path "$LOCAL_SCRATCH_DIR")
openbao_data_dir=$(resolve_path "$OPENBAO_DATA_DIR")
recovery_dir=$(resolve_path "$OPENBAO_RECOVERY_DIR")
backup_dir=$(resolve_path "$OPENBAO_BACKUP_DIR")
backup_approle_file=$(resolve_path "$OPENBAO_BACKUP_APPROLE_FILE")

ensure_api_directory "$media_dir"
ensure_api_directory "$scratch_dir"
ensure_openbao_data_directory "$openbao_data_dir"
for path in "$recovery_dir" "$backup_dir" "$(dirname -- "$backup_approle_file")"; do
  ensure_operator_directory "$path"
done

realm="$root/keycloak/realm-voxis-oss.json"
sed -e "s|__OSS_PUBLIC_URL__|${PUBLIC_FRONTEND_URL}|g" \
  -e "s|__MCP_RESOURCE_AUDIENCE__|${MCP_RESOURCE_AUDIENCE}|g" \
  "$root/keycloak/realm-template.json" > "$realm"
chmod 600 "$env_file"
chmod 644 "$realm"
# Containers read these mounted files as their own non-root users, so a
# restrictive umask at checkout must not hide them.
chmod 644 "$root/openbao/openbao.hcl" "$root/caddy/Caddyfile" "$root/caddy/Caddyfile.smoke"
chmod 755 "$root/postgres/init-app-role.sh"
bash "$root/scripts/load-apparmor-profile.sh"

printf 'Bootstrap complete. Build the images, then start only Keycloak and OpenBao:\n'
printf '  docker compose --env-file %s -f %s --profile app build\n' "$env_file" "$root/compose.yaml"
printf '  docker compose --env-file %s -f %s up -d --no-build --wait --wait-timeout 240 keycloak openbao\n' "$env_file" "$root/compose.yaml"
printf 'Then continue the install sequence in %s. OpenBao is intentionally not initialized by this script.\n' "$root/README.md"
printf 'To restore a backup here instead, build the images and run restore.sh without starting any service.\n'
