#!/usr/bin/env bash

# OpenBao helpers shared by the operator scripts. Callers define the `compose`
# array. Tokens, unseal shares, and AppRole secrets travel over standard
# input: never in command arguments or the Docker CLI environment.

# The application AppRole may only be used from the Compose `secrets` network
# (see compose.yaml). The backup AppRole may only be used from inside the
# OpenBao container, where backup.sh runs the bao CLI.
openbao_app_cidr=172.30.2.0/24
openbao_backup_cidr=127.0.0.1/32

# Legacy root-generation endpoints are enabled only on this loopback listener
# inside the OpenBao container (openbao/openbao.hcl).
openbao_recovery_addr=http://127.0.0.1:8210

# bao_as TOKEN ARGS...: run the bao CLI as TOKEN with no other input.
bao_as() {
  local token=$1
  shift
  printf '%s\n' "$token" | "${compose[@]}" exec -T openbao \
    sh -c 'IFS= read -r BAO_TOKEN; export BAO_TOKEN; exec bao "$@"' bao "$@"
}

# bao_as_with_input TOKEN ARGS...: like bao_as, passing the caller's standard
# input after the token line.
bao_as_with_input() {
  local token=$1
  shift
  { printf '%s\n' "$token"; cat; } | "${compose[@]}" exec -T openbao \
    sh -c 'IFS= read -r BAO_TOKEN; export BAO_TOKEN; exec bao "$@"' bao "$@"
}

# openbao_status: print `bao status` JSON. Exit status 2 (sealed) is normal.
openbao_status() {
  "${compose[@]}" exec -T openbao bao status -format=json || [[ $? -eq 2 ]]
}

# openbao_read_shares COUNT [FILE]: fill the `unseal_shares` array with COUNT
# shares, from FILE (one share per line) or typed without echo.
openbao_read_shares() {
  local count=$1 file=${2:-} share index
  [[ "$count" =~ ^[1-9][0-9]*$ ]] || { echo 'Invalid OpenBao unseal threshold.' >&2; return 1; }
  unseal_shares=()
  if [[ -n "$file" ]]; then
    [[ -f "$file" && -r "$file" ]] || { echo "Cannot read the unseal share file: $file" >&2; return 1; }
    while IFS= read -r share || [[ -n "$share" ]]; do
      share=${share//[[:space:]]/}
      [[ -n "$share" ]] && unseal_shares+=("$share")
      (( ${#unseal_shares[@]} < count )) || break
    done < "$file"
  else
    for (( index = 1; index <= count; index++ )); do
      read -r -s -p "OpenBao unseal share ${index}/${count}: " share
      printf '\n' >&2
      unseal_shares+=("$share")
    done
  fi
  (( ${#unseal_shares[@]} == count )) || {
    echo "OpenBao needs ${count} unseal shares." >&2
    return 1
  }
}

# openbao_unseal: submit the shares in `unseal_shares`.
openbao_unseal() {
  local share
  for share in "${unseal_shares[@]}"; do
    printf '%s' "$share" | "${compose[@]}" exec -T openbao bao write sys/unseal key=- >/dev/null
  done
  openbao_status | grep -Eq '"sealed"[[:space:]]*:[[:space:]]*false' || {
    echo 'OpenBao is still sealed; check the unseal shares.' >&2
    return 1
  }
}

# openbao_recovery_request PATH: POST standard input to a root-generation
# endpoint on the loopback listener and print the JSON response.
openbao_recovery_request() {
  "${compose[@]}" exec -T openbao wget -q -O - --header 'Content-Type: application/json' \
    --post-file=/dev/stdin "$openbao_recovery_addr/v1/sys/generate-root/$1"
}

# openbao_decode_root ENCODED OTP: undo the one-time-password XOR.
openbao_decode_root() {
  local encoded=$1 otp=$2 index=0 byte otp_byte hex=''
  local -a bytes
  while (( ${#encoded} % 4 )); do encoded+='='; done
  read -r -a bytes < <(printf '%s' "$encoded" | base64 -d | od -An -v -tu1 | tr -s ' \n' '  ')
  (( ${#bytes[@]} == ${#otp} )) || { echo 'Cannot decode the generated root token.' >&2; return 1; }
  for byte in "${bytes[@]}"; do
    printf -v otp_byte '%d' "'${otp:index:1}"
    printf -v hex '%s\\x%02x' "$hex" $(( byte ^ otp_byte ))
    index=$(( index + 1 ))
  done
  printf '%b\n' "$hex"
}

# openbao_generate_root: print a temporary root token made from the shares in
# `unseal_shares`. The caller must revoke it with openbao_revoke.
openbao_generate_root() {
  local attempt nonce otp share update encoded
  attempt=$(printf '{}' | openbao_recovery_request attempt) || {
    echo 'Cannot start root token generation. Cancel a stale attempt first.' >&2
    return 1
  }
  nonce=$(jq -er '.nonce' <<<"$attempt")
  otp=$(jq -er '.otp' <<<"$attempt")
  for share in "${unseal_shares[@]}"; do
    update=$(jq -cn --arg key "$share" --arg nonce "$nonce" '{key: $key, nonce: $nonce}' \
      | openbao_recovery_request update)
  done
  jq -e '.complete == true' <<<"$update" >/dev/null || {
    echo 'Root token generation did not complete; check the unseal shares.' >&2
    openbao_cancel_root_generation
    return 1
  }
  encoded=$(jq -er '.encoded_token' <<<"$update")
  openbao_decode_root "$encoded" "$otp"
}

# openbao_cancel_root_generation: discard an unfinished root generation.
openbao_cancel_root_generation() {
  "${compose[@]}" exec -T -e "BAO_ADDR=$openbao_recovery_addr" openbao \
    bao delete sys/generate-root/attempt >/dev/null 2>&1 || true
}

# openbao_revoke TOKEN: revoke a token; it must no longer work afterwards.
openbao_revoke() {
  bao_as "$1" token revoke -self >/dev/null
  if bao_as "$1" token lookup >/dev/null 2>&1; then
    echo 'OpenBao token is still valid after revocation.' >&2
    return 1
  fi
}

# openbao_configure_voxis ROOT_TOKEN: create the transit engine, the narrow
# policies, and the two AppRoles on a freshly initialized store.
openbao_configure_voxis() {
  local token=$1
  # The API creates org-* keys and exports them. `+` matches only the key
  # name, so rotate, config, trim, and delete stay out of reach.
  bao_as_with_input "$token" policy write voxis-transit - >/dev/null <<'POLICY'
path "transit/keys/+" {
  capabilities = ["create", "update"]
  allowed_parameters = {
    "type" = ["aes256-gcm96"]
    "exportable" = []
  }
}
path "transit/export/encryption-key/org-*" {
  capabilities = ["read"]
}
POLICY
  bao_as_with_input "$token" policy write voxis-backup - >/dev/null <<'POLICY'
path "sys/storage/raft/snapshot" {
  capabilities = ["read"]
}
POLICY
  bao_as "$token" secrets enable transit >/dev/null
  bao_as "$token" auth enable approle >/dev/null
  # secret_id_ttl=0 keeps the application secret ID valid until it is
  # rotated with rotate-approle-secret.sh.
  bao_as "$token" write auth/approle/role/voxis-transit token_policies=voxis-transit \
    token_ttl=1h token_max_ttl=24h secret_id_ttl=0 \
    secret_id_bound_cidrs="$openbao_app_cidr" token_bound_cidrs="$openbao_app_cidr" >/dev/null
  bao_as "$token" write auth/approle/role/voxis-backup token_policies=voxis-backup \
    token_ttl=1h token_max_ttl=1h secret_id_ttl=0 \
    secret_id_bound_cidrs="$openbao_backup_cidr" token_bound_cidrs="$openbao_backup_cidr" >/dev/null
}

# openbao_issue_secret_id TOKEN ROLE: print the role ID, a new secret ID, and
# its accessor as JSON. Older secret IDs stay valid until destroyed.
openbao_issue_secret_id() {
  local token=$1 role=$2 role_id secret
  role_id=$(bao_as "$token" read -field=role_id "auth/approle/role/$role/role-id")
  secret=$(bao_as "$token" write -format=json -f "auth/approle/role/$role/secret-id")
  jq -ec --arg role_id "$role_id" \
    '{role_id: $role_id, secret_id: .data.secret_id, accessor: .data.secret_id_accessor}' <<<"$secret"
}

# openbao_destroy_other_secret_ids TOKEN ROLE KEEP_ACCESSOR: destroy every
# secret ID of ROLE except the one with KEEP_ACCESSOR.
openbao_destroy_other_secret_ids() {
  local token=$1 role=$2 keep=$3 accessors accessor
  accessors=$(bao_as "$token" list -format=json "auth/approle/role/$role/secret-id")
  jq -e --arg keep "$keep" 'any(.[]; . == $keep)' <<<"$accessors" >/dev/null || {
    echo "The new $role secret ID is missing; refusing to destroy the others." >&2
    return 1
  }
  for accessor in $(jq -r --arg keep "$keep" '.[] | select(. != $keep)' <<<"$accessors"); do
    printf '{"secret_id_accessor":"%s"}' "$accessor" \
      | bao_as_with_input "$token" write "auth/approle/role/$role/secret-id-accessor/destroy" - >/dev/null
  done
  accessors=$(bao_as "$token" list -format=json "auth/approle/role/$role/secret-id")
  jq -e --arg keep "$keep" '. == [$keep]' <<<"$accessors" >/dev/null || {
    echo "Older $role secret IDs remain after destruction." >&2
    return 1
  }
}

# openbao_write_app_approle ENV_FILE CREDENTIALS_JSON: store the application
# AppRole in .env without placing the secret in a command argument.
openbao_write_app_approle() {
  local env_file=$1 credentials=$2 env_tmp
  env_tmp=$(mktemp "${env_file}.tmp.XXXXXX")
  if ! VAULT_ROLE_ID=$(jq -r '.role_id' <<<"$credentials") \
    VAULT_SECRET_ID=$(jq -r '.secret_id' <<<"$credentials") awk '
    /^VAULT_ROLE_ID=/ { print "VAULT_ROLE_ID=" ENVIRON["VAULT_ROLE_ID"]; next }
    /^VAULT_SECRET_ID=/ { print "VAULT_SECRET_ID=" ENVIRON["VAULT_SECRET_ID"]; next }
    { print }
  ' "$env_file" > "$env_tmp"; then
    rm -f "$env_tmp"
    return 1
  fi
  chmod 600 "$env_tmp"
  mv "$env_tmp" "$env_file"
}

# openbao_write_backup_approle FILE CREDENTIALS_JSON: store the backup AppRole.
openbao_write_backup_approle() {
  local file=$1 credentials=$2
  ( umask 077; mkdir -p "$(dirname -- "$file")"; jq -c '{role_id, secret_id}' <<<"$credentials" > "$file" )
  chmod 600 "$file"
}
