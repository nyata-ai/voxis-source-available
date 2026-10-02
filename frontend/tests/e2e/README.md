# Voxis Source-Available browser tests

Run these tests against a completed local or staged OSS installation. The test account must already belong to a disposable organization and have the administrator role. The suite creates a browser session only; it does not create accounts or use a shared login.

Set these values before `npx playwright test`:

- `VOXIS_OSS_E2E_BASE_URL`, for example `https://localhost`.
- `VOXIS_OSS_E2E_ISSUER`, ending in `/auth/realms/voxis-oss`.
- `VOXIS_OSS_E2E_USERNAME` and `VOXIS_OSS_E2E_PASSWORD` for the installation-specific test account.
- `VOXIS_OSS_E2E_TOTP_SECRET`, the test account's unpadded Base32 one-time-password secret. The bundled realm asks for a one-time code at every sign-in.

Set `VOXIS_OSS_E2E_START_COMMAND` only when the test runner should start that local installation. It must serve the same value as `VOXIS_OSS_E2E_BASE_URL`.

Run these checks only on the disposable acceptance VM. The browser uses the
installation's trusted local certificate; do not disable TLS checks.

## Trusting Caddy's local certificate

With `PUBLIC_HOSTNAME=localhost`, or any name without a public certificate,
Caddy signs the site with its own local certificate authority. Playwright's
Chromium on Linux trusts the certificates in the NSS database under
`$HOME/.pki/nssdb`, so add Caddy's root there. Use a separate `HOME` so this
trust never reaches your own browser profile. From `infra/oss`:

```bash
export HOME=/tmp/voxis-e2e-home
mkdir -p "$HOME/.pki/nssdb"
docker compose --env-file .env -f compose.yaml --profile app \
  cp proxy:/data/caddy/pki/authorities/local/root.crt "$HOME/caddy-root.crt"
certutil -d "sql:$HOME/.pki/nssdb" -N --empty-password
certutil -d "sql:$HOME/.pki/nssdb" -A -t 'C,,' -n voxis-caddy-local -i "$HOME/caddy-root.crt"
```

`certutil` comes from the NSS tools package (`libnss3-tools` on Debian). The
acceptance specs also call the API from Node through Playwright's request
context, which ignores the NSS database, so trust the same root there too:

```bash
export NODE_EXTRA_CA_CERTS="$HOME/caddy-root.crt"
```

Run `npx playwright test` from `frontend` in the same shell. Set
`PLAYWRIGHT_BROWSERS_PATH` if the browsers were installed under your normal
home directory. Delete the test `HOME` afterwards. Each new or restored
installation has its own Caddy root, so repeat these steps for it.

## Synthetic acceptance fixture

Build the short spoken WAV locally on the VM, with no network or provider
request:

```bash
bash tests/e2e/fixtures/generate-synthetic-meeting-wav.sh --output /tmp/voxis-acceptance/synthetic-meeting.wav --dry-run
bash tests/e2e/fixtures/generate-synthetic-meeting-wav.sh --output /tmp/voxis-acceptance/synthetic-meeting.wav
ffmpeg -hide_banner -loglevel error -y -i /tmp/voxis-acceptance/synthetic-meeting.wav -c:a aac -f adts /tmp/voxis-acceptance/synthetic-meeting.aac
```

Record the JSON line from the second command in the private acceptance record.
It contains the fixture path, SHA-256, and duration. The WAV is synthetic and
at most 60 seconds. It is the only audio that may be used if a later, separate
provider permission is granted.

Before the full-stack check, prepare a private VM-only fixture through the
running application's normal encrypted storage and database paths. Keep its
result file mode 600. Read that result file only to set these variables for the
browser run:

- `VOXIS_OSS_ACCEPTANCE_MEDIA_ID`
- `VOXIS_OSS_ACCEPTANCE_TRANSCRIPTION_ID`
- `VOXIS_OSS_ACCEPTANCE_OVERSIZED_TRANSCRIPTION_ID`
- `VOXIS_OSS_ACCEPTANCE_LATE_SEGMENT_ID`

Do not commit the result file or print its values. The normal fixture must have
four source chunks within the configured 20 KiB limit. Its final chunk must
contain the `limited pilot` decision and the `retention schedule` action so the
summary check can confirm late-source coverage. It must also provide the final
chunk's segment ID. Prepare a separate valid oversized transcript whose source
exceeds that limit. The repository does not ship a reusable transcript seeder:
keep an installation-specific acceptance seeder and its generated IDs outside
the public source tree.

## Focused browser checks

```bash
VOXIS_OSS_RECORDING_E2E=1 npx playwright test tests/e2e/recording-recovery.acceptance.spec.ts
VOXIS_OSS_SCAN_E2E=1 VOXIS_OSS_ACCEPTANCE_WAV_PATH=/tmp/voxis-acceptance/synthetic-meeting.wav VOXIS_OSS_ACCEPTANCE_AAC_PATH=/tmp/voxis-acceptance/synthetic-meeting.aac VOXIS_OSS_ACCEPTANCE_SCANNER_POSITIVE_WAV_PATH=/tmp/voxis-acceptance/scanner-positive-control.wav npx playwright test tests/e2e/scan-gating.acceptance.spec.ts
VOXIS_OSS_FULL_STACK_E2E=1 npx playwright test tests/e2e/full-stack.acceptance.spec.ts
```

The recording test uses Chrome's synthetic microphone device but sends real
MediaRecorder chunks through the application, reloads the page, recovers the
session, and waits until stitched media is clean and range-playable. The scan
test requires a clean WAV to stream, accepts the Windows AAC MIME alias, and
requires a scanner-positive media control to stay quarantined. On the
disposable VM, create that control as a distinct benign WAV and load a
temporary, task-local ClamAV hash signature for it. Reload the scanner, prove
that it detects the file directly, then run the test. Remove the temporary
signature and reload the scanner after the test. A missing or incomplete
scanner must fail the test.

The full-stack check uses the authenticated browser session to create and
revoke an API key, call MCP `initialize`, `tools/list`, and `get_media`, and
exercise the saved transcript, range stream, search, edit, exports, standard
summary, structured summary, size rejection, statistics, and administrator
page. It disables traces, screenshots, and video for that test because the
one-time API key never belongs in a report.

Fixture setup and these checks do not send audio to a provider. A provider run
needs separate, explicit approval and must delete its synthetic transcription
and media afterward.
