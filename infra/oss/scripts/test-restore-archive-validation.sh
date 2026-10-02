#!/usr/bin/env bash
set -euo pipefail

command -v age >/dev/null || { echo 'age is required.' >&2; exit 1; }
command -v age-keygen >/dev/null || { echo 'age-keygen is required.' >&2; exit 1; }

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=lib/restore_archive.sh
. "$root/scripts/lib/restore_archive.sh"

tmp=$(mktemp -d)
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT
age-keygen -o "$tmp/identity.txt" >/dev/null
recipient=$(age-keygen -y "$tmp/identity.txt")
export BACKUP_AGE_IDENTITY="$tmp/identity.txt"

decrypt_and_validate() {
  local name=$1
  restore_decrypt "$tmp/$name.tar.age" "$tmp/$name.decrypted.tar" || return 1
  [[ $(stat -c '%a' "$tmp/$name.decrypted.tar") == 600 ]] || { echo 'Decrypted archive must be private.' >&2; exit 1; }
  restore_validate_tar "$tmp/$name.decrypted.tar"
}

mkdir "$tmp/safe"
printf 'encrypted media metadata only\n' > "$tmp/safe/record.txt"
tar -C "$tmp/safe" -cf "$tmp/safe.tar" .
age -r "$recipient" -o "$tmp/safe.tar.age" "$tmp/safe.tar"
decrypt_and_validate safe

expect_refusal() {
  local name=$1
  if decrypt_and_validate "$name" >/dev/null 2>&1; then
    echo "Expected $name archive to be refused." >&2
    exit 1
  fi
}

ln -s record.txt "$tmp/safe/symlink"
tar -C "$tmp/safe" -cf "$tmp/symlink.tar" symlink
age -r "$recipient" -o "$tmp/symlink.tar.age" "$tmp/symlink.tar"
expect_refusal symlink

ln "$tmp/safe/record.txt" "$tmp/safe/hardlink"
tar -C "$tmp/safe" -cf "$tmp/hardlink.tar" record.txt hardlink
age -r "$recipient" -o "$tmp/hardlink.tar.age" "$tmp/hardlink.tar"
expect_refusal hardlink

mkfifo "$tmp/safe/fifo"
tar -C "$tmp/safe" -cf "$tmp/fifo.tar" fifo
age -r "$recipient" -o "$tmp/fifo.tar.age" "$tmp/fifo.tar"
expect_refusal fifo

tar -C "$tmp/safe" --transform='s|record.txt|../escape.txt|' -cf "$tmp/traversal.tar" record.txt
age -r "$recipient" -o "$tmp/traversal.tar.age" "$tmp/traversal.tar"
expect_refusal traversal

tar -C "$tmp/safe" --transform='s|record.txt|/absolute.txt|' -cf "$tmp/absolute.tar" record.txt
age -r "$recipient" -o "$tmp/absolute.tar.age" "$tmp/absolute.tar"
expect_refusal absolute

printf 'not an age file\n' > "$tmp/corrupt.tar.age"
expect_refusal corrupt

# keycloak_dump CLIENT_REALM_ID: a minimal plain pg_dump with two realms and
# one voxis-oss-api-lookup client in the given realm (none when empty).
keycloak_dump() {
  printf 'COPY public.client (id, enabled, client_id, realm_id, secret) FROM stdin;\n'
  printf 'c1\tt\tvoxis-oss-web\tr1\t\\N\n'
  [[ -z $1 ]] || printf 'c2\tt\tvoxis-oss-api-lookup\t%s\tsecret\n' "$1"
  printf '%s\n\n' '\.'
  printf 'COPY public.realm (id, access_code_lifespan, "name", enabled) FROM stdin;\n'
  printf 'r1\t60\tvoxis-oss\tt\nr2\t60\tmaster\tt\n'
  printf '%s\n' '\.'
}
keycloak_dump r1 | restore_realm_has_lookup_client voxis-oss || {
  echo 'A realm with the user-lookup client was refused.' >&2
  exit 1
}
if keycloak_dump '' | restore_realm_has_lookup_client voxis-oss; then
  echo 'A pre-release realm without the user-lookup client was accepted.' >&2
  exit 1
fi
if keycloak_dump r2 | restore_realm_has_lookup_client voxis-oss; then
  echo 'A user-lookup client in another realm was accepted.' >&2
  exit 1
fi

echo 'Restore archive validation decrypts once into a private file, accepts regular files/directories, and refuses unsafe paths, links, special members, undecryptable input, and pre-release Keycloak realms without the user-lookup client.'
