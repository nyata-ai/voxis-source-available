#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
env_file="$root/.env"
[[ -f "$env_file" ]] || { echo 'Missing .env.' >&2; exit 1; }

privileged() {
  if [[ $EUID -eq 0 ]]; then
    "$@"
    return
  fi
  command -v sudo >/dev/null || { echo 'backup.sh requires sudo to read API-owned media.' >&2; return 1; }
  sudo "$@"
}
recipient_from_shell=${BACKUP_AGE_RECIPIENT:-}
set -a
# shellcheck disable=SC1090
. "$env_file"
set +a
BACKUP_AGE_RECIPIENT=${BACKUP_AGE_RECIPIENT:-$recipient_from_shell}
[[ "$BACKUP_AGE_RECIPIENT" =~ ^age1[0-9a-z]+$ ]] || { echo 'Set BACKUP_AGE_RECIPIENT in .env to an age public recipient (age1...).' >&2; exit 1; }
command -v age >/dev/null || { echo 'age is required for encrypted backups.' >&2; exit 1; }
compose=(docker compose --env-file "$env_file" -f "$root/compose.yaml")
if [[ -n ${VOXIS_OSS_COMPOSE_OVERRIDE_FILES:-} ]]; then
  IFS=: read -r -a compose_overrides <<< "$VOXIS_OSS_COMPOSE_OVERRIDE_FILES"
  for compose_override in "${compose_overrides[@]}"; do
    [[ -n "$compose_override" ]] || { echo 'Compose override path is empty.' >&2; exit 1; }
    [[ -f "$compose_override" ]] || { echo "Compose override is missing: $compose_override" >&2; exit 1; }
    compose+=(-f "$compose_override")
  done
fi
resolve_path() {
  case "$1" in
    /*) printf '%s\n' "$1" ;;
    *) printf '%s/%s\n' "$root" "$1" ;;
  esac
}

media_dir=$(resolve_path "$LOCAL_STORAGE_DIR")
backup_root=$(resolve_path "$OPENBAO_BACKUP_DIR")
backup_approle_file=$(resolve_path "$OPENBAO_BACKUP_APPROLE_FILE")
command -v jq >/dev/null || { echo 'jq is required.' >&2; exit 1; }
# shellcheck source=lib/openbao.sh
. "$root/scripts/lib/openbao.sh"
[[ -r "$backup_approle_file" ]] || { echo 'The OpenBao backup AppRole file is missing or unreadable.' >&2; exit 1; }
# Cleanup state. The EXIT trap is installed before the OpenBao login, so the
# token is revoked on every later failure, and writers are restarted only if
# this script stopped them.
backup_token=''
api_container=''
keycloak_container=''
snapshot_path=/tmp/voxis-oss-backup.snap
snapshot_created=0

wait_for_keycloak_discovery() {
  local realm=${OSS_KEYCLOAK_REALM:-voxis-oss}
  [[ "$realm" =~ ^[A-Za-z0-9_-]+$ ]] || {
    printf 'Invalid Keycloak realm name for discovery readiness.\n' >&2
    return 1
  }
  for _ in $(seq 1 45); do
    if "${compose[@]}" exec -T -e "OSS_KEYCLOAK_REALM=$realm" keycloak bash -lc '
      realm=${OSS_KEYCLOAK_REALM:?}
      exec 3<>/dev/tcp/127.0.0.1/8080
      printf "GET /auth/realms/%s/.well-known/openid-configuration HTTP/1.1\\r\\nHost: keycloak\\r\\nConnection: close\\r\\n\\r\\n" "$realm" >&3
      IFS=" " read -r protocol status _ <&3
      [[ "$protocol" == HTTP/* && "$status" == 200 ]]
    ' >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  printf 'Keycloak discovery did not become ready after restart.\n' >&2
  return 1
}

wait_for_api_ready() {
  for _ in $(seq 1 60); do
    if "${compose[@]}" --profile app exec -T api curl --fail --silent --show-error http://127.0.0.1:8080/ready >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  printf 'API did not become ready after restart.\n' >&2
  return 1
}

restore_services() {
  local status=$?
  local resume_failed=0
  trap - EXIT
  if [[ $snapshot_created -eq 1 ]]; then
    if ! "${compose[@]}" exec -T openbao rm -f "$snapshot_path" >/dev/null 2>&1; then
      printf 'Failed to remove the temporary OpenBao snapshot at %s.\n' "$snapshot_path" >&2
      resume_failed=1
    fi
  fi
  if [[ -n "$keycloak_container" ]]; then
    if ! "${compose[@]}" start keycloak >/dev/null; then
      printf 'Failed to resume the previously running Keycloak service.\n' >&2
      resume_failed=1
    elif ! wait_for_keycloak_discovery; then
      printf 'Keycloak started but discovery was not ready.\n' >&2
      resume_failed=1
    fi
  fi
  if [[ -n "$api_container" ]]; then
    if [[ $resume_failed -ne 0 ]]; then
      printf 'API was not restarted because required identity recovery failed.\n' >&2
    elif ! "${compose[@]}" --profile app start api >/dev/null; then
      printf 'Failed to resume the previously running API service.\n' >&2
      resume_failed=1
    elif ! wait_for_api_ready; then
      printf 'API started but readiness did not recover.\n' >&2
      resume_failed=1
    fi
  fi
  if [[ -n "$backup_token" ]] && ! openbao_revoke "$backup_token"; then
    printf 'Failed to revoke the temporary OpenBao backup token.\n' >&2
    resume_failed=1
  fi
  unset backup_token
  if [[ $resume_failed -ne 0 ]]; then
    printf 'Backup writer recovery failed; inspect service state before relying on this backup.\n' >&2
    exit 1
  fi
  exit "$status"
}
trap restore_services EXIT

# The backup AppRole can only snapshot Raft and only logs in from inside the
# OpenBao container. Its one-hour token is revoked when this script exits.
backup_token=$(jq -ec '{role_id, secret_id}' "$backup_approle_file" \
  | "${compose[@]}" exec -T openbao bao write -field=token auth/approle/login -)
[[ -n "$backup_token" ]] || { echo 'The OpenBao backup login returned no token.' >&2; exit 1; }
stamp=$(date -u +%Y%m%dT%H%M%SZ)
umask 077
out="$backup_root/$stamp"
mkdir -p "$out"

# Remember only writers that were running. A backup must not start a service
# that an operator had deliberately stopped before it began.
api_container=$("${compose[@]}" ps --status running -q api 2>/dev/null || true)
keycloak_container=$("${compose[@]}" ps --status running -q keycloak 2>/dev/null || true)

# Stop all writers before any database, media, or Raft backup starts. The API
# runs its River workers, and Keycloak owns its own database writes.
if [[ -n "$api_container" ]]; then
  "${compose[@]}" --profile app stop api >/dev/null
fi
if [[ -n "$keycloak_container" ]]; then
  "${compose[@]}" stop keycloak >/dev/null
fi

"${compose[@]}" exec -T postgres pg_dump -U "$POSTGRES_APP_USER" "$POSTGRES_DB" | age -r "$BACKUP_AGE_RECIPIENT" -o "$out/voxis.sql.age"
"${compose[@]}" exec -T keycloak-db pg_dump -U "$KEYCLOAK_DB_USER" "$KEYCLOAK_DB" | age -r "$BACKUP_AGE_RECIPIENT" -o "$out/keycloak.sql.age"
bao_as "$backup_token" operator raft snapshot save "$snapshot_path" >/dev/null
snapshot_created=1
"${compose[@]}" exec -T openbao cat "$snapshot_path" | age -r "$BACKUP_AGE_RECIPIENT" -o "$out/openbao.snap.age"
"${compose[@]}" exec -T openbao rm -f "$snapshot_path"
snapshot_created=0
privileged tar -C "$media_dir" -cf - . | age -r "$BACKUP_AGE_RECIPIENT" -o "$out/media.tar.age"
(
  cd "$out"
  sha256sum -- *.age > SHA256SUMS
)
printf 'Encrypted backup written to %s\n' "$out"
printf 'It holds no OpenBao unseal shares; a restore needs the custodians. It is encrypted but not signed: keep it on write-once, off-site storage.\n'
