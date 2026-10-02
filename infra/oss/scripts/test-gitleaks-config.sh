#!/usr/bin/env bash
set -euo pipefail

root=$(git -C "$(dirname -- "$0")/../.." rev-parse --show-toplevel)
scanner=${GITLEAKS_BIN:-gitleaks}
fixture=$(mktemp -d)

cleanup() {
  rm -rf -- "$fixture"
}
trap cleanup EXIT

git -C "$fixture" init --quiet
mkdir -p "$fixture/frontend/src/i18n/locales/es"
cp "$root/.gitleaks.toml" "$fixture/.gitleaks.toml"
setting_key=$(printf '%s%s' update Password)
translation=$(printf '%s%s' 'Actualizar/Cambiar ' 'contraseña')
printf '    "%s": "%s",\n' "$setting_key" "$translation" > "$fixture/frontend/src/i18n/locales/es/settings.json"
# The derived Keycloak image tags are allowlisted by path and full line, so
# they must pass in any commit and in a current-tree scan.
mkdir -p "$fixture/infra/oss" "$fixture/editions/oss"
grep -x 'VOXIS_OSS_KEYCLOAK_IMAGE=.*' "$root/infra/oss/.env.example" > "$fixture/infra/oss/.env.example"
grep -x 'VOXIS_OSS_KEYCLOAK_IMAGE=.*' "$root/editions/oss/.env.ci" > "$fixture/editions/oss/.env.ci"
git -C "$fixture" add .
git -C "$fixture" -c user.name='Gitleaks fixture' -c user.email='gitleaks@example.invalid' commit --quiet -m 'add allowed translation'

"$scanner" detect --source "$fixture" --config "$fixture/.gitleaks.toml" --log-opts="--all" --redact=100 --no-banner >/dev/null 2>&1 || {
  printf 'The documented false positives are not allowlisted.\n' >&2
  exit 1
}

(
  cd "$fixture"
  "$scanner" detect --source . --no-git --config .gitleaks.toml --redact=100 --no-banner >/dev/null 2>&1
) || {
  printf 'The documented false positives are not allowlisted for a current-tree scan.\n' >&2
  exit 1
}

suffix=$(printf '%s%s%s%s' 'A1b2C3d4E5' 'F6g7H8i9J0' 'K1l2M3n4O5' 'P6q7R8s9T0')
generic_suffix=$(printf '%s' 'voxis oss gitleaks generic fixture' | sha256sum | cut -c1-32)
printf '    "api_key": "ghp_%s"\n' "$suffix" >> "$fixture/frontend/src/i18n/locales/es/settings.json"
printf '    "credential": "%s"\n' "$generic_suffix" >> "$fixture/frontend/src/i18n/locales/es/settings.json"
git -C "$fixture" add frontend/src/i18n/locales/es/settings.json
git -C "$fixture" -c user.name='Gitleaks fixture' -c user.email='gitleaks@example.invalid' commit --quiet -m 'add synthetic secret'

set +e
"$scanner" detect --source "$fixture" --config "$fixture/.gitleaks.toml" --log-opts="--all" --redact=100 --report-format json --report-path "$fixture/findings.json" --no-banner >/dev/null 2>&1
scan_status=$?
set -e
[[ $scan_status -eq 1 ]] || {
  printf 'The synthetic secret scan ended with %s instead of a leak finding.\n' "$scan_status" >&2
  exit 1
}

set +e
(
  cd "$fixture"
  "$scanner" detect --source . --no-git --config .gitleaks.toml --redact=100 --report-format json --report-path "$fixture/findings-no-git.json" --no-banner >/dev/null 2>&1
)
no_git_status=$?
set -e
[[ $no_git_status -eq 1 ]] || {
  printf 'The no-Git synthetic secret scan ended with %s instead of a leak finding.\n' "$no_git_status" >&2
  exit 1
}

python3 - "$fixture/findings.json" "$fixture/findings-no-git.json" <<'PY'
import json
import sys

expected_path = "frontend/src/i18n/locales/es/settings.json"
expected = {
    (expected_path, "github-pat"),
    (expected_path, "generic-api-key"),
}
allowed_overlap = {(expected_path, "github-oauth")}
for report in sys.argv[1:]:
    findings = json.load(open(report, encoding="utf-8"))
    actual = {(str(finding.get("File", "")).replace("\\", "/").lstrip("./"), finding.get("RuleID")) for finding in findings}
    if expected - actual or actual - expected - allowed_overlap:
        raise SystemExit("The synthetic secrets did not produce the expected redacted findings.")
PY
