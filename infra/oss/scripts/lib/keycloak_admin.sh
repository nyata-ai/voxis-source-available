#!/usr/bin/env bash

# Keycloak admin REST helpers shared by the configure-keycloak-*.sh scripts.
# Callers define the `compose` array. The operator signs in as a master-realm
# administrator (with a one-time code once OTP is set up). Credentials and
# tokens reach curl inside the Keycloak container through its configuration on
# standard input, never through command arguments.

kc_base=http://localhost:8080/auth

# kc_escape VALUE: quote VALUE for a curl configuration string.
kc_escape() {
  local value=${1//\\/\\\\}
  printf '%s' "${value//\"/\\\"}"
}

# kc_curl: run curl in the Keycloak container with configuration on stdin.
kc_curl() {
  "${compose[@]}" exec -T keycloak curl --silent --show-error --config -
}

# kc_store_session JSON: keep the tokens from a token-endpoint response.
kc_store_session() {
  kc_access_token=$(jq -er '.access_token' <<<"$1")
  kc_refresh_token=$(jq -er '.refresh_token' <<<"$1")
  kc_token_time=$SECONDS
}

# kc_admin_login: prompt for a master-realm administrator and sign in.
kc_admin_login() {
  local user password otp config response
  read -r -p 'Keycloak master-realm administrator: ' user
  read -r -s -p 'Password: ' password
  printf '\n' >&2
  read -r -p 'One-time code (leave empty if OTP is not set up yet): ' otp
  config="url = \"$kc_base/realms/master/protocol/openid-connect/token\"
fail
data-urlencode = \"grant_type=password\"
data-urlencode = \"client_id=admin-cli\"
data-urlencode = \"username=$(kc_escape "$user")\"
data-urlencode = \"password=$(kc_escape "$password")\""
  [[ -z "$otp" ]] || config+=$'\n'"data-urlencode = \"totp=$(kc_escape "$otp")\""
  unset password otp
  response=$(printf '%s\n' "$config" | kc_curl) || {
    echo 'Keycloak administrator sign-in failed.' >&2
    return 1
  }
  unset config
  kc_store_session "$response"
}

# kc_refresh: renew the short-lived admin access token before it expires.
kc_refresh() {
  local response
  (( SECONDS - kc_token_time < 30 )) && return 0
  response=$(printf '%s\n' "url = \"$kc_base/realms/master/protocol/openid-connect/token\"
fail
data-urlencode = \"grant_type=refresh_token\"
data-urlencode = \"client_id=admin-cli\"
data-urlencode = \"refresh_token=$kc_refresh_token\"" | kc_curl) || {
    echo 'Keycloak administrator session could not be renewed.' >&2
    return 1
  }
  kc_store_session "$response"
}

# kc_admin METHOD PATH [JSON]: call /admin/realms/PATH and print the body.
kc_admin() {
  local method=$1 path=$2 body=${3:-} config response code
  kc_refresh
  config="url = \"$kc_base/admin/realms/$path\"
request = \"$method\"
header = \"Authorization: Bearer $kc_access_token\"
header = \"Content-Type: application/json\"
write-out = \"\\n%{http_code}\""
  [[ -z "$body" ]] || config+=$'\n'"data-binary = \"$(kc_escape "$body")\""
  response=$(printf '%s\n' "$config" | kc_curl)
  code=${response##*$'\n'}
  [[ "$code" == 2?? ]] || {
    echo "Keycloak admin $method $path returned HTTP $code." >&2
    return 1
  }
  printf '%s' "${response%$'\n'*}"
}

# kc_admin_logout: end the administrator session.
kc_admin_logout() {
  [[ -n "${kc_refresh_token:-}" ]] || return 0
  printf '%s\n' "url = \"$kc_base/realms/master/protocol/openid-connect/logout\"
data-urlencode = \"client_id=admin-cli\"
data-urlencode = \"refresh_token=$kc_refresh_token\"" | kc_curl >/dev/null 2>&1 || true
  unset kc_access_token kc_refresh_token
}
