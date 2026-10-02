#!/usr/bin/env bash

# Archive validation shared by restore.sh and its focused refusal test. The
# restore identity is intentionally supplied by the caller through
# BACKUP_AGE_IDENTITY. Each archive is decrypted once into a private file; the
# checks and the extraction then read that file.

# restore_decrypt ARCHIVE OUTPUT: decrypt ARCHIVE into OUTPUT with mode 600.
restore_decrypt() {
  local archive=$1 output=$2
  ( umask 077; age --decrypt -i "$BACKUP_AGE_IDENTITY" -o "$output" "$archive" ) || {
    echo "Cannot decrypt archive: $archive" >&2
    return 1
  }
}

restore_validate_member_paths() {
  local tarball=$1
  local member
  local members
  if ! members=$(tar -tf "$tarball"); then
    echo "Cannot read archive: $tarball" >&2
    return 1
  fi
  [[ -n "$members" ]] || { echo "Backup archive has no members: $tarball" >&2; return 1; }

  while IFS= read -r member; do
    [[ "$member" == . || "$member" == ./ ]] && continue
    member=${member#./}
    case "/$member/" in
      //*) echo "Backup archive has an absolute member path: $member" >&2; return 1 ;;
      */../*) echo "Backup archive has an unsafe member path: $member" >&2; return 1 ;;
    esac
  done <<<"$members"
}

restore_validate_member_types() {
  local tarball=$1
  local entry
  local listing
  if ! listing=$(tar -tvf "$tarball"); then
    echo "Cannot inspect archive types: $tarball" >&2
    return 1
  fi
  [[ -n "$listing" ]] || { echo "Backup archive has no members: $tarball" >&2; return 1; }

  while IFS= read -r entry; do
    case "${entry:0:1}" in
      -|d) ;;
      *) echo "Backup archive has an unsupported member type: ${entry:0:1}" >&2; return 1 ;;
    esac
  done <<<"$listing"
}

# restore_validate_tar TARBALL: accept only relative regular files and
# directories in an already decrypted archive.
restore_validate_tar() {
  local tarball=$1
  restore_validate_member_paths "$tarball" || return 1
  restore_validate_member_types "$tarball" || return 1
}

# restore_realm_has_lookup_client REALM: succeed when the plain pg_dump of the
# Keycloak database on standard input has a voxis-oss-api-lookup client in
# REALM. Backups from earlier pre-release candidates lack it, and this release
# imports the realm only on a fresh first start, so it cannot add it later.
restore_realm_has_lookup_client() {
  local realm=$1
  awk -v realm="$realm" -v client='voxis-oss-api-lookup' '
    function columns(header, index_of,   list, count, i, names) {
      list = header
      sub(/^[^(]*\(/, "", list)
      sub(/\) FROM stdin;$/, "", list)
      gsub(/"/, "", list)
      count = split(list, names, /, /)
      for (i = 1; i <= count; i++) index_of[names[i]] = i
    }
    /^COPY public\.realm \(/ { columns($0, realm_col); table = "realm"; next }
    /^COPY public\.client \(/ { columns($0, client_col); table = "client"; next }
    $0 == "\\." { table = ""; next }
    table == "realm" {
      split($0, field, "\t")
      if (field[realm_col["name"]] == realm) realm_id = field[realm_col["id"]]
      next
    }
    table == "client" {
      split($0, field, "\t")
      if (field[client_col["client_id"]] == client) client_realm[field[client_col["realm_id"]]] = 1
      next
    }
    END { exit !(realm_id != "" && (realm_id in client_realm)) }
  '
}
