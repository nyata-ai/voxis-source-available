#!/usr/bin/env bash
set -euo pipefail

usage='usage: VOXIS_RESTORE_CONFIRM=RESTORE_FRESH_TARGET restore.sh BACKUP_DIR [UNSEAL_SHARES_FILE]'
backup_dir=${1:?$usage}
shares_file=${2:-}
[[ ${VOXIS_RESTORE_CONFIRM:-} == RESTORE_FRESH_TARGET ]] || {
  echo 'Set VOXIS_RESTORE_CONFIRM=RESTORE_FRESH_TARGET. This command restores only a fresh target.' >&2
  exit 1
}
[[ -n ${BACKUP_AGE_IDENTITY:-} ]] || { echo 'Set BACKUP_AGE_IDENTITY to an age identity file.' >&2; exit 1; }
command -v age >/dev/null || { echo 'age is required.' >&2; exit 1; }
command -v jq >/dev/null || { echo 'jq is required to read OpenBao responses.' >&2; exit 1; }
command -v docker >/dev/null || { echo 'docker is required.' >&2; exit 1; }
if [[ -n "$shares_file" ]]; then
  shares_file=$(CDPATH= cd -- "$(dirname -- "$shares_file")" && pwd)/$(basename -- "$shares_file")
  [[ -f "$shares_file" && -r "$shares_file" ]] || { echo "Cannot read the unseal share file: $shares_file" >&2; exit 1; }
fi

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=lib/restore_archive.sh
. "$root/scripts/lib/restore_archive.sh"
# shellcheck source=lib/openbao.sh
. "$root/scripts/lib/openbao.sh"
env_file="$root/.env"
[[ -f "$env_file" ]] || { echo 'Run bootstrap.sh in the fresh restore target first.' >&2; exit 1; }

privileged() {
  if [[ $EUID -eq 0 ]]; then
    "$@"
    return
  fi
  command -v sudo >/dev/null || { echo 'restore.sh requires sudo to restore API-owned media.' >&2; return 1; }
  sudo "$@"
}
backup_dir=$(CDPATH= cd -- "$backup_dir" && pwd)
set -a
# shellcheck disable=SC1090
. "$env_file"
set +a
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
scratch_dir=$(resolve_path "$LOCAL_SCRATCH_DIR")
openbao_data_dir=$(resolve_path "$OPENBAO_DATA_DIR")
backup_approle_file=$(resolve_path "$OPENBAO_BACKUP_APPROLE_FILE")

for member in voxis.sql.age keycloak.sql.age media.tar.age openbao.snap.age SHA256SUMS; do
  [[ -f "$backup_dir/$member" ]] || { echo "Backup member is missing: $member" >&2; exit 1; }
done
if [[ -e "$backup_dir/openbao-recovery.tar.age" ]]; then
  echo 'This backup predates the custodian model and carries OpenBao recovery material. It is ignored here; destroy that file and older copies of it.' >&2
fi
bash "$root/scripts/verify-backup.sh" "$backup_dir"
# Refuse a backup from an earlier pre-release candidate before anything is
# started or changed: its Keycloak realm lacks the user-lookup client, and the
# realm is imported only on a fresh first start.
set +e
age --decrypt -i "$BACKUP_AGE_IDENTITY" "$backup_dir/keycloak.sql.age" \
  | restore_realm_has_lookup_client "${OSS_KEYCLOAK_REALM:-voxis-oss}"
realm_check=("${PIPESTATUS[@]}")
set -e
[[ ${realm_check[0]} -eq 0 ]] || { echo 'Cannot decrypt keycloak.sql.age with BACKUP_AGE_IDENTITY. Nothing was changed.' >&2; exit 1; }
[[ ${realm_check[1]} -eq 0 ]] || {
  echo 'This backup was made with an earlier pre-release version: its Keycloak realm has no voxis-oss-api-lookup client. Restoring it is not supported; install this release fresh instead. Nothing was changed.' >&2
  exit 1
}

empty_directory_required() {
  local directory=$1 entries
  [[ -d "$directory" ]] || { echo "Fresh target directory is missing: $directory" >&2; exit 1; }
  entries=$(privileged find "$directory" -mindepth 1 -maxdepth 1 -print -quit) || {
    echo "Cannot inspect fresh target directory: $directory" >&2
    exit 1
  }
  if [[ -n "$entries" ]]; then
    echo "Fresh target directory is not empty: $directory" >&2
    exit 1
  fi
}

require_openbao_data_owner() {
  local owner mode
  owner=$(privileged stat -c '%u:%g' "$openbao_data_dir") || {
    echo 'Cannot read fresh OpenBao data owner.' >&2
    exit 1
  }
  mode=$(privileged stat -c '%a' "$openbao_data_dir") || {
    echo 'Cannot read fresh OpenBao data permissions.' >&2
    exit 1
  }
  [[ "$owner" == 100:1000 && "$mode" == 700 ]] || {
    echo 'Fresh OpenBao data must be owned by 100:1000 with mode 700; rerun bootstrap.sh.' >&2
    exit 1
  }
}

for directory in "$media_dir" "$scratch_dir" "$openbao_data_dir"; do
  empty_directory_required "$directory"
done
require_openbao_data_owner
[[ ! -e "$backup_approle_file" ]] || { echo "Fresh target already has a backup AppRole file: $backup_approle_file" >&2; exit 1; }

for service in postgres keycloak-db keycloak openbao api web proxy; do
  [[ -z $("${compose[@]}" ps -aq "$service" 2>/dev/null || true) ]] || {
    echo "Fresh restore target already has a $service container; refusing to overwrite it." >&2
    exit 1
  }
done
[[ -z $(docker volume ls --quiet --filter "label=com.docker.compose.project=$COMPOSE_PROJECT_NAME") ]] || {
  echo 'Fresh restore target already has Compose volumes; refusing to overwrite them.' >&2
  exit 1
}

# The media archive is decrypted once into a private working directory, checked,
# and extracted from there. It needs free space equal to the media archive.
work_dir=$(mktemp -d)
chmod 700 "$work_dir"
temporary_snapshot=/tmp/voxis-oss-restore.snap
restore_root_token=''
# Set once the restore starts changing this target. A failure after that
# leaves it partly restored, and a plain rerun would refuse it.
target_changed=false
cleanup() {
  local status=$?
  trap - EXIT
  rm -rf "$work_dir"
  "${compose[@]}" exec -T openbao rm -f "$temporary_snapshot" >/dev/null 2>&1 || true
  if [[ -n "$restore_root_token" ]] && ! openbao_revoke "$restore_root_token"; then
    echo 'Failed to revoke the temporary OpenBao root token.' >&2
    status=1
  fi
  unset target_root_token restore_root_token unseal_shares app_credentials backup_credentials
  if [[ $status -ne 0 && $target_changed == true ]]; then
    echo "The restore failed and left this target partly restored. Follow 'Retrying a failed restore' in $root/README.md before running it again." >&2
  fi
  exit "$status"
}
trap cleanup EXIT

restore_decrypt "$backup_dir/media.tar.age" "$work_dir/media.tar"
restore_validate_tar "$work_dir/media.tar"

wait_for_postgres() {
  for _ in $(seq 1 45); do
    if "${compose[@]}" exec -T postgres pg_isready -U "$POSTGRES_APP_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo 'PostgreSQL did not become ready for restore.' >&2
  return 1
}

wait_for_keycloak_db() {
  for _ in $(seq 1 45); do
    if "${compose[@]}" exec -T keycloak-db pg_isready -U "$KEYCLOAK_DB_USER" -d "$KEYCLOAK_DB" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo 'Keycloak PostgreSQL did not become ready for restore.' >&2
  return 1
}

wait_for_keycloak_discovery() {
  local realm=${OSS_KEYCLOAK_REALM:-voxis-oss}
  [[ "$realm" =~ ^[A-Za-z0-9_-]+$ ]] || {
    echo 'Invalid Keycloak realm name for discovery readiness.' >&2
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
  echo 'Keycloak discovery did not become ready after restore.' >&2
  return 1
}

wait_for_api_ready() {
  for _ in $(seq 1 60); do
    if "${compose[@]}" --profile app exec -T api curl --fail --silent --show-error http://127.0.0.1:8080/ready >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo 'API did not become ready after restore.' >&2
  return 1
}

wait_for_openbao() {
  local pattern=$1
  for _ in $(seq 1 45); do
    openbao_status 2>/dev/null | grep -Eq "$pattern" && return 0
    sleep 1
  done
  return 1
}

target_changed=true
"${compose[@]}" up -d postgres keycloak-db openbao >/dev/null
wait_for_postgres
wait_for_keycloak_db
wait_for_openbao '"initialized"[[:space:]]*:[[:space:]]*false' || { echo 'OpenBao did not become an uninitialized fresh store.' >&2; exit 1; }

# A one-share temporary initialization gives OpenBao a token that may restore
# the encrypted source snapshot. The snapshot replaces that store, its shares,
# and its token. A host administrator who controls Docker or the host can
# observe this recovery operation.
target_init=$("${compose[@]}" exec -T openbao bao operator init -key-shares=1 -key-threshold=1 -format=json)
target_root_token=$(jq -er '.root_token' <<<"$target_init")
mapfile -t unseal_shares < <(jq -r '.unseal_keys_b64[]' <<<"$target_init")
unset target_init
openbao_unseal

restore_decrypt "$backup_dir/openbao.snap.age" "$work_dir/openbao.snap"
"${compose[@]}" exec -T openbao sh -ec "umask 077; cat > '$temporary_snapshot'" < "$work_dir/openbao.snap"
rm -f "$work_dir/openbao.snap"
bao_as "$target_root_token" operator raft snapshot restore -force "$temporary_snapshot" >/dev/null
unset target_root_token
"${compose[@]}" exec -T openbao rm -f "$temporary_snapshot"
"${compose[@]}" restart openbao >/dev/null
wait_for_openbao '"sealed"[[:space:]]*:[[:space:]]*true' || { echo 'OpenBao did not restart as the sealed source store.' >&2; exit 1; }

# The restored store needs the source custodians: the same threshold of shares
# unseals it and then generates a temporary root token, revoked on exit.
threshold=$(openbao_status | jq -er '.t')
echo "The restored OpenBao store needs ${threshold} of its unseal shares." >&2
openbao_read_shares "$threshold" "$shares_file"
openbao_unseal
restore_root_token=$(openbao_generate_root)

# Issue new secret IDs on this host and destroy those issued to the source host.
app_credentials=$(openbao_issue_secret_id "$restore_root_token" voxis-transit)
openbao_destroy_other_secret_ids "$restore_root_token" voxis-transit "$(jq -r '.accessor' <<<"$app_credentials")"
openbao_write_app_approle "$env_file" "$app_credentials"
backup_credentials=$(openbao_issue_secret_id "$restore_root_token" voxis-backup)
openbao_destroy_other_secret_ids "$restore_root_token" voxis-backup "$(jq -r '.accessor' <<<"$backup_credentials")"
openbao_write_backup_approle "$backup_approle_file" "$backup_credentials"
unset app_credentials backup_credentials
openbao_revoke "$restore_root_token"
restore_root_token=''
# The original .env values were exported before restore. Docker Compose gives
# those process values precedence over the restored file, so clear only them
# before creating the API.
unset VAULT_ROLE_ID VAULT_SECRET_ID

age --decrypt -i "$BACKUP_AGE_IDENTITY" "$backup_dir/voxis.sql.age" \
  | "${compose[@]}" exec -T postgres psql -v ON_ERROR_STOP=1 -U "$POSTGRES_APP_USER" -d "$POSTGRES_DB" >/dev/null
age --decrypt -i "$BACKUP_AGE_IDENTITY" "$backup_dir/keycloak.sql.age" \
  | "${compose[@]}" exec -T keycloak-db psql -v ON_ERROR_STOP=1 -U "$KEYCLOAK_DB_USER" -d "$KEYCLOAK_DB" >/dev/null
# The restored realm keeps the source host's lookup-client secret. Replace it
# with this host's generated value before Keycloak starts.
[[ "$KEYCLOAK_USER_LOOKUP_CLIENT_SECRET" =~ ^[0-9a-f]{64}$ ]] || { echo 'KEYCLOAK_USER_LOOKUP_CLIENT_SECRET must be the value generated by bootstrap.sh.' >&2; exit 1; }
[[ "${OSS_KEYCLOAK_REALM:-voxis-oss}" =~ ^[A-Za-z0-9_-]+$ ]] || { echo 'Invalid Keycloak realm name.' >&2; exit 1; }
updated=$(printf "UPDATE client SET secret = '%s' WHERE client_id = 'voxis-oss-api-lookup' AND realm_id = (SELECT id FROM realm WHERE name = '%s');\n" \
  "$KEYCLOAK_USER_LOOKUP_CLIENT_SECRET" "${OSS_KEYCLOAK_REALM:-voxis-oss}" \
  | "${compose[@]}" exec -T keycloak-db psql -v ON_ERROR_STOP=1 -U "$KEYCLOAK_DB_USER" -d "$KEYCLOAK_DB" -At)
[[ "$updated" == 'UPDATE 1' ]] || { echo 'Could not set the Keycloak user-lookup client secret.' >&2; exit 1; }
privileged tar -C "$media_dir" --no-same-owner --no-same-permissions -xf "$work_dir/media.tar"
rm -f "$work_dir/media.tar"
privileged chown -R 10001:10001 "$media_dir"
privileged chmod 0700 "$media_dir"

"${compose[@]}" up -d keycloak >/dev/null
wait_for_keycloak_discovery
"${compose[@]}" --profile app up -d api >/dev/null
wait_for_api_ready
echo "Fresh restore completed from $backup_dir"
echo 'The source host AppRole secret IDs are destroyed. Verify an encrypted record and the restored Keycloak users before admitting traffic.'
