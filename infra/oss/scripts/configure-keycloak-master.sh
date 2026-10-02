#!/usr/bin/env bash
set -euo pipefail

# Hardens the Keycloak master (administration) realm: brute-force detection,
# a password policy, login and admin events, a one-time password for every new
# administrator, and a frontend URL on the loopback admin port so the admin
# console never signs in through the public proxy. It does not change users,
# clients, or the Voxis realm.
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
env_file="$root/.env"
[[ -f "$env_file" ]] || { echo 'Run bootstrap.sh first.' >&2; exit 1; }
command -v jq >/dev/null || { echo 'jq is required to configure Keycloak.' >&2; exit 1; }
set -a
# shellcheck disable=SC1090
. "$env_file"
set +a
admin_port=${OSS_KEYCLOAK_ADMIN_PORT:-8081}
[[ "$admin_port" =~ ^[0-9]+$ ]] || { echo 'OSS_KEYCLOAK_ADMIN_PORT must be a port number.' >&2; exit 1; }
compose=(docker compose --env-file "$env_file" -f "$root/compose.yaml")
# shellcheck source=lib/keycloak_admin.sh
. "$root/scripts/lib/keycloak_admin.sh"

settings='{
  "bruteForceProtected": true,
  "passwordPolicy": "length(12) and notUsername and notEmail and passwordHistory(3)",
  "eventsEnabled": true,
  "eventsExpiration": 7776000,
  "adminEventsEnabled": true,
  "adminEventsDetailsEnabled": true
}'
# Realm attributes are replaced as a whole, and tokens carry the frontend URL
# as issuer, so the attributes are set last: that update ends this session.
frontend=$(jq -cn --arg frontend "http://localhost:$admin_port/auth" \
  '{attributes: {adminEventsExpiration: "7776000", frontendUrl: $frontend}}')

kc_admin_login
trap kc_admin_logout EXIT

kc_admin PUT master "$(jq -c . <<<"$settings")" >/dev/null
action=$(kc_admin GET master/authentication/required-actions/CONFIGURE_TOTP)
kc_admin PUT master/authentication/required-actions/CONFIGURE_TOTP \
  "$(jq -c '.enabled = true | .defaultAction = true' <<<"$action")" >/dev/null

realm=$(kc_admin GET master)
jq -e --argjson want "$settings" '
  .bruteForceProtected == true and
  .passwordPolicy == $want.passwordPolicy and
  .eventsEnabled == true and .eventsExpiration == $want.eventsExpiration and
  .adminEventsEnabled == true and .adminEventsDetailsEnabled == true
' <<<"$realm" >/dev/null || { echo 'The master realm settings did not apply.' >&2; exit 1; }
kc_admin GET master/authentication/required-actions/CONFIGURE_TOTP \
  | jq -e '.enabled == true and .defaultAction == true' >/dev/null || {
  echo 'The master realm does not require OTP for new administrators.' >&2
  exit 1
}
kc_admin PUT master "$frontend" >/dev/null
echo "Hardened the Keycloak master realm: brute-force detection, password policy, 90-day login and admin events, OTP for new administrators, and sign-in only at http://localhost:$admin_port/auth."
