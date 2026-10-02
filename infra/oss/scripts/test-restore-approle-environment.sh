#!/usr/bin/env bash
set -euo pipefail

command -v docker >/dev/null || { echo 'docker is required.' >&2; exit 1; }
command -v jq >/dev/null || { echo 'jq is required.' >&2; exit 1; }
command -v python3 >/dev/null || { echo 'python3 is required.' >&2; exit 1; }

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
restore_script="$root/scripts/restore.sh"
tmp=$(mktemp -d)
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT

cat > "$tmp/.env" <<'EOF'
VAULT_ROLE_ID=old-file-role
VAULT_SECRET_ID=old-file-secret
EOF
cat > "$tmp/compose.yaml" <<'EOF'
services:
  api:
    image: example.invalid/voxis-test
    environment:
      VAULT_ROLE_ID: ${VAULT_ROLE_ID:-}
      VAULT_SECRET_ID: ${VAULT_SECRET_ID:-}
EOF

set -a
# shellcheck disable=SC1090
. "$tmp/.env"
set +a
sed -i 's/^VAULT_ROLE_ID=.*/VAULT_ROLE_ID=restored-file-role/; s/^VAULT_SECRET_ID=.*/VAULT_SECRET_ID=restored-file-secret/' "$tmp/.env"

old_config=$(docker compose --env-file "$tmp/.env" -f "$tmp/compose.yaml" config)
[[ "$old_config" == *'VAULT_ROLE_ID: old-file-role'* ]] || {
  echo 'Test setup did not reproduce process-environment precedence.' >&2
  exit 1
}
[[ "$old_config" == *'VAULT_SECRET_ID: old-file-secret'* ]] || {
  echo 'Test setup did not reproduce process-environment precedence.' >&2
  exit 1
}

unset VAULT_ROLE_ID VAULT_SECRET_ID
restored_config=$(docker compose --env-file "$tmp/.env" -f "$tmp/compose.yaml" config)
[[ "$restored_config" == *'VAULT_ROLE_ID: restored-file-role'* ]] || {
  echo 'Compose did not use the restored AppRole ID.' >&2
  exit 1
}
[[ "$restored_config" == *'VAULT_SECRET_ID: restored-file-secret'* ]] || {
  echo 'Compose did not use the restored AppRole secret.' >&2
  exit 1
}

for script in "$restore_script" "$root/scripts/rotate-approle-secret.sh"; do
  update_line=$(grep -n 'openbao_write_app_approle "\$env_file"' "$script" | head -n1 | cut -d: -f1)
  clear_line=$(grep -n '^unset VAULT_ROLE_ID VAULT_SECRET_ID$' "$script" | cut -d: -f1)
  api_line=$(grep -n 'profile app up -d.* api' "$script" | cut -d: -f1)
  [[ "$update_line" =~ ^[0-9]+$ && "$clear_line" =~ ^[0-9]+$ && "$api_line" =~ ^[0-9]+$ \
    && "$update_line" -lt "$clear_line" && "$clear_line" -lt "$api_line" ]] || {
    echo "$(basename "$script") must clear stale AppRole environment values after writing .env and before API creation." >&2
    exit 1
  }
done

# Drive rotate-approle-secret.sh against a fake Docker CLI that plays OpenBao.
fake_bin="$tmp/fake-bin"
state="$tmp/state"
test_root="$tmp/openbao-scripts"
mkdir -p "$fake_bin" "$state" "$test_root/scripts/lib"
cp "$root/scripts/rotate-approle-secret.sh" "$test_root/scripts/"
cp "$root/scripts/lib/openbao.sh" "$test_root/scripts/lib/"
cat > "$test_root/.env" <<'EOF'
COMPOSE_PROJECT_NAME=voxis-oss-test
VAULT_ROLE_ID=retained-role
VAULT_SECRET_ID=previous-secret
UNRELATED_ENV=retained
EOF
printf 'services: {}\n' > "$test_root/compose.yaml"
printf 'fake-share-one\nfake-share-two\nfake-share-three\n' > "$tmp/shares"
fake_root='fake-generated-root-token'
fake_otp='abcdefghijklmnopqrstuvwxy'
encoded=$(python3 -c 'import base64,sys; t,o=sys.argv[1].encode(),sys.argv[2].encode(); print(base64.b64encode(bytes(a^b for a,b in zip(t,o))).decode().rstrip("="))' "$fake_root" "$fake_otp")
printf '["previous-accessor","new-accessor"]' > "$state/accessors"

cat > "$fake_bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$FAKE_STATE/argv"
args=" $* "
case "$args" in
  *' bao status '*) printf '{"sealed":false,"t":3}\n'; exit 0 ;;
  *'/sys/generate-root/attempt'*) cat >/dev/null; printf '{"nonce":"fake-nonce","otp":"%s"}\n' "$FAKE_OTP"; exit 0 ;;
  *'/sys/generate-root/update'*)
    cat >> "$FAKE_STATE/shares-seen"
    printf '{"complete":true,"encoded_token":"%s"}\n' "$FAKE_ENCODED"; exit 0 ;;
  *' up -d '*)
    printf 'VAULT_SECRET_ID=%s\n' "${VAULT_SECRET_ID-unset}" >> "$FAKE_STATE/env"
    printf 'up\n' >> "$FAKE_STATE/order"; exit 0 ;;
  *' exec -T api '*) printf 'ready\n' >> "$FAKE_STATE/order"; exit 0 ;;
esac
if [[ "$args" == *' exec -T openbao sh -c '* ]]; then
  IFS= read -r token
  printf '%s\n' "$token" >> "$FAKE_STATE/tokens"
  case "$args" in
    *' read -field=role_id '*) printf 'retained-role\n' ;;
    *' write -format=json -f auth/approle/role/voxis-transit/secret-id '*)
      printf '{"data":{"secret_id":"%s","secret_id_accessor":"new-accessor"}}\n' "$FAKE_APPROLE_SECRET" ;;
    *' list -format=json auth/approle/role/voxis-transit/secret-id '*) cat "$FAKE_STATE/accessors" ;;
    *'/secret-id-accessor/destroy '*)
      jq -r '.secret_id_accessor' >> "$FAKE_STATE/destroyed"
      printf 'destroy\n' >> "$FAKE_STATE/order"
      printf '["new-accessor"]' > "$FAKE_STATE/accessors" ;;
    *' token revoke -self '*) printf 'revoked\n' >> "$FAKE_STATE/order" ;;
    *' token lookup '*) exit 2 ;;
    *) echo "unexpected bao call: $*" >&2; exit 1 ;;
  esac
  exit 0
fi
echo "unexpected docker call: $*" >&2
exit 1
EOF
chmod 700 "$fake_bin/docker"

export FAKE_STATE="$state" FAKE_OTP="$fake_otp" FAKE_ENCODED="$encoded"
export FAKE_APPROLE_SECRET='fake-approle-secret'
PATH="$fake_bin:$PATH" bash "$test_root/scripts/rotate-approle-secret.sh" "$tmp/shares" >/dev/null

grep -Fx 'VAULT_ROLE_ID=retained-role' "$test_root/.env" >/dev/null
grep -Fx "VAULT_SECRET_ID=$FAKE_APPROLE_SECRET" "$test_root/.env" >/dev/null
grep -Fx 'UNRELATED_ENV=retained' "$test_root/.env" >/dev/null
[[ $(stat -c '%a' "$test_root/.env") == 600 ]] || { echo 'Updated AppRole environment file must have mode 600.' >&2; exit 1; }
[[ $(<"$state/destroyed") == previous-accessor ]] || { echo 'Rotation did not destroy exactly the previous secret ID.' >&2; exit 1; }
[[ $(tr '\n' ' ' < "$state/order") == 'up ready destroy revoked ' ]] || {
  echo "Rotation must reload the API, confirm readiness, destroy the old secret ID, then revoke root: $(tr '\n' ' ' < "$state/order")" >&2
  exit 1
}
grep -Fx "$fake_root" "$state/tokens" >/dev/null || { echo 'The decoded root token did not reach bao over standard input.' >&2; exit 1; }
[[ $(grep -c 'fake-share' "$state/shares-seen") -eq 3 ]] || { echo 'Root generation did not receive three shares.' >&2; exit 1; }
if grep -F 'VAULT_SECRET_ID=previous-secret' "$state/env" >/dev/null; then
  echo 'The stale AppRole secret reached the Docker CLI call that recreates the API.' >&2
  exit 1
fi
for secret in "$fake_root" "$FAKE_APPROLE_SECRET" fake-share-one fake-share-two fake-share-three "$fake_otp"; do
  if grep -Fq -- "$secret" "$state/argv"; then
    echo 'An OpenBao token, share, or AppRole secret reached Docker arguments.' >&2
    exit 1
  fi
done

openbao_scripts=(
  "$root/scripts/init-openbao.sh"
  "$root/scripts/init-openbao-smoke.sh"
  "$root/scripts/backup.sh"
  "$root/scripts/rotate-approle-secret.sh"
  "$root/scripts/restore.sh"
  "$root/scripts/bootstrap.sh"
  "$root/scripts/lib/openbao.sh"
)
for script in "${openbao_scripts[@]}"; do
  if grep -Eq -- '-e BAO_TOKEN|BAO_TOKEN="\$' "$script"; then
    echo "An OpenBao token is passed through the Docker CLI environment: $script" >&2
    exit 1
  fi
  if grep -Fq -- 'sed -i "s|^' "$script"; then
    echo "Generated secret remains in sed arguments: $script" >&2
    exit 1
  fi
  if grep -Fq -- 'bao operator unseal' "$script"; then
    echo "Unseal share remains in arguments: $script" >&2
    exit 1
  fi
done
grep -Fq -- 'bao write sys/unseal key=-' "$root/scripts/lib/openbao.sh" || {
  echo 'Standard-input unseal is missing from lib/openbao.sh.' >&2
  exit 1
}

echo 'Restore and rotation clear stale AppRole process values, rotation destroys the old secret ID only after the API is ready, and OpenBao credentials stay out of command arguments.'
