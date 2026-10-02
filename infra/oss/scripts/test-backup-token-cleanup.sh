#!/usr/bin/env bash
# Drive backup.sh against a fake Docker CLI to check that the OpenBao backup
# token is revoked when the backup fails right after login, and that no revoke
# is attempted when the login itself fails.
set -euo pipefail

command -v jq >/dev/null || { echo 'jq is required.' >&2; exit 1; }

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT

test_root="$tmp/oss"
fake_bin="$tmp/fake-bin"
state="$tmp/state"
mkdir -p "$test_root/scripts/lib" "$fake_bin" "$state" "$tmp/media"
cp "$root/scripts/backup.sh" "$test_root/scripts/"
cp "$root/scripts/lib/openbao.sh" "$test_root/scripts/lib/"
printf 'services: {}\n' > "$test_root/compose.yaml"
printf '{"role_id":"fake-role","secret_id":"fake-secret"}\n' > "$tmp/backup-approle.json"
# A regular file where the backup directory should be makes `mkdir -p` fail
# after the login, which is the early failure under test.
printf 'not a directory\n' > "$tmp/backups"
cat > "$test_root/.env" <<EOF
BACKUP_AGE_RECIPIENT=age1fakerecipient
LOCAL_STORAGE_DIR=$tmp/media
OPENBAO_BACKUP_DIR=$tmp/backups
OPENBAO_BACKUP_APPROLE_FILE=$tmp/backup-approle.json
EOF

cat > "$fake_bin/age" <<'EOF'
#!/usr/bin/env bash
cat >/dev/null
EOF
cat > "$fake_bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$FAKE_STATE/argv"
case " $* " in
  *' auth/approle/login '*)
    cat >/dev/null
    [[ ${FAKE_LOGIN_FAILS:-0} -eq 1 ]] && exit 2
    printf 'fake-backup-token\n'
    ;;
  *' token revoke -self '*)
    IFS= read -r token
    printf '%s\n' "$token" >> "$FAKE_STATE/revoked"
    ;;
  *' token lookup '*) exit 2 ;;
  *' ps '*) ;;
  *) printf 'unexpected docker call: %s\n' "$*" >&2; exit 97 ;;
esac
EOF
chmod +x "$fake_bin/age" "$fake_bin/docker"

run_backup() {
  PATH="$fake_bin:$PATH" FAKE_STATE="$state" bash "$test_root/scripts/backup.sh" >/dev/null 2>"$state/stderr"
}

# Case 1: login succeeds, the backup directory cannot be created.
if run_backup; then
  echo 'backup.sh succeeded although the backup directory could not be created.' >&2
  exit 1
fi
[[ -f "$state/revoked" && "$(cat "$state/revoked")" == 'fake-backup-token' ]] || {
  echo 'backup.sh did not revoke the OpenBao backup token after an early failure.' >&2
  cat "$state/stderr" >&2
  exit 1
}

# Case 2: the login fails, so there is no token to revoke.
rm -f "$state/revoked" "$state/argv"
if FAKE_LOGIN_FAILS=1 run_backup; then
  echo 'backup.sh succeeded although the OpenBao login failed.' >&2
  exit 1
fi
[[ ! -e "$state/revoked" ]] || {
  echo 'backup.sh tried to revoke a token although the login failed.' >&2
  exit 1
}
if grep -Eq ' (start|stop) ' "$state/argv"; then
  echo 'backup.sh stopped or started services although the login failed.' >&2
  exit 1
fi
printf 'backup token cleanup: ok\n'
