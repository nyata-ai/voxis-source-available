#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source_profile="$root/security/bwrap-moby-v29.2.0-apparmor.profile"
profile_name=voxis-oss-api-bwrap
installed_profile="/etc/apparmor.d/$profile_name"
profile_sha256=50ef346d12d4009ce0cc825d0e71f4567ea6df6db4da6b5cd62b33fddd0d1d99

privileged() {
  if [[ $EUID -eq 0 ]]; then
    "$@"
    return
  fi
  command -v sudo >/dev/null || {
    echo 'AppArmor profile installation requires root or sudo.' >&2
    return 1
  }
  sudo "$@"
}

[[ -r "$source_profile" ]] || {
  echo "Missing AppArmor profile: $source_profile" >&2
  exit 1
}
[[ $(sha256sum "$source_profile" | awk '{print $1}') == "$profile_sha256" ]] || {
  echo "AppArmor profile checksum does not match its reviewed provenance." >&2
  exit 1
}
parser_path=$(privileged sh -c 'command -v apparmor_parser') || {
  echo 'AppArmor tooling is unavailable; refusing to start the API without its required profile.' >&2
  exit 1
}
[[ -r /sys/module/apparmor/parameters/enabled ]] || {
  echo 'AppArmor is unavailable; refusing to start the API without its required profile.' >&2
  exit 1
}
[[ $(< /sys/module/apparmor/parameters/enabled) == Y ]] || {
  echo 'AppArmor is disabled; refusing to start the API without its required profile.' >&2
  exit 1
}

privileged install -D -m 0644 "$source_profile" "$installed_profile"
privileged "$parser_path" --replace --write-cache "$installed_profile"
privileged grep -Fxq "$profile_name (enforce)" /sys/kernel/security/apparmor/profiles || {
  echo "AppArmor did not load $profile_name in enforce mode." >&2
  exit 1
}

printf 'Loaded AppArmor profile %s.\n' "$profile_name"
