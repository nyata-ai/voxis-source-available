#!/usr/bin/env bash
# Checks the first-sign-in contract of the imported voxis-oss realm. Keycloak
# registers only the required actions a realm import lists, so an action left
# out is silently ignored: a temporary password would never have to change.
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
realm="$root/keycloak/realm-template.json"

jq -e '
  [.requiredActions[] | select(.enabled)] as $enabled
  | ($enabled | any(.providerId == "UPDATE_PASSWORD"))
  and ($enabled | any(.providerId == "CONFIGURE_TOTP" and .defaultAction))
' "$realm" >/dev/null || {
  printf 'The voxis-oss realm must register UPDATE_PASSWORD and a default CONFIGURE_TOTP.\n' >&2
  exit 1
}
printf 'Realm template registers UPDATE_PASSWORD and a default CONFIGURE_TOTP.\n'

# MCP requires the email and email_verified claims. A dynamically registered
# client that omits `scope` gets the realm default scopes; one that sends
# `scope` keeps only `basic` as a default scope in Keycloak 26.7, so `basic`
# must carry both claims in the access token.
jq -e '.defaultDefaultClientScopes | index("basic") != null and index("email") != null' "$realm" >/dev/null || {
  printf 'The voxis-oss realm must give new clients the basic and email default scopes.\n' >&2
  exit 1
}
jq -e '
  [.clientScopes[] | select(.name == "basic") | .protocolMappers[]
    | select(.config["access.token.claim"] == "true") | .config["claim.name"]] as $claims
  | ($claims | index("email") != null) and ($claims | index("email_verified") != null)
' "$realm" >/dev/null || {
  printf 'The basic client scope must put email and email_verified in access tokens.\n' >&2
  exit 1
}
printf 'Realm template gives every client the email and email_verified claims.\n'
