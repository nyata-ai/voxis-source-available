# Voxis Source-Available local runtime

This Compose template runs the whole stack on one operator-owned host:
PostgreSQL, Keycloak, OpenBao Raft storage, ClamAV, local encrypted media, and a
local Gemma runtime. The `app` profile adds the API and web images plus Caddy,
which owns ports 80 and 443 and obtains TLS certificates.

Compose builds the API and web images from the reviewed edition sources. Record
their resulting digests in the release manifest before deployment.

Each PostgreSQL container initializes a bootstrap superuser and a separate
application database owner. The API and Keycloak receive only the application
credentials. Keep the bootstrap passwords in the private `.env` file; normal
application operation, migrations, backups, and restores do not require them.

## Fresh installation required

This release must be installed fresh. Upgrading an installation made with an
earlier pre-release candidate is not supported, and neither is restoring a
backup made with one. Keycloak imports the realm only on its first start, so an
older installation keeps an older realm that lacks settings this release needs.
`restore.sh` checks the backup's realm and stops, before it changes anything,
if the backup comes from an earlier candidate.

## Host prerequisites

Use a Linux host with Docker Engine, the Docker Compose plugin, Bash, `jq`,
OpenSSL, `age`, and AppArmor tooling including `apparmor_parser`. A normal
operator also needs Docker access and `sudo`; a root-operated host runs the
same commands directly. The API media sandbox has been tested on Debian 12
with AppArmor 3.0.8. Other Linux distributions need the same AppArmor loader
and media-sandbox checks before use. Docker Desktop alone is not the supported
host for the full sandbox.

The initial runtime supports Linux x86_64/amd64 only. The reviewed API, Caddy,
Gosu, and DeepFilter runtime inputs are amd64 builds.

Turn swap off on the host, or encrypt it. OpenBao cannot lock its memory with
the Raft storage it uses, so unencrypted swap could write key material to disk.

## Measured host shape

The recorded default-model run used eight CPU threads and 31.35 GiB of RAM. It
had 22.02 GiB available before the stack started. In a steady sample, Gemma
used 7.21 GiB; ClamAV, Keycloak, OpenBao, the API, databases, web server, and
proxy together used about 2.24 GiB. Treat this as the tested starting point,
not a lower limit or a concurrency promise.

ClamAV runs as two services from the same image. `clamav` (the clamd scanner)
has a 3 GiB memory limit. It used about 0.95 GiB in a steady sample and
briefly holds a second copy of its signatures while it loads an update.
`freshclam` (signature updates) has a 2 GiB limit, because it loads each
downloaded database once to test it. Gemma has no memory limit: its use is set
by the model and the fixed context size.

The verified model volume is 6.50 GiB. Keep it separate from the upload reserve
below and from Docker's image and build cache. The acceptance run reused that
model volume, so it did not measure the peak extra space for a first image
build. Check free space and Docker cache use before the first build instead of
assuming the upload reserve covers them.

## Local upload capacity

The default configuration allows four uploads of up to 1 GiB each. If
`LOCAL_STORAGE_DIR` and `LOCAL_SCRATCH_DIR` share a filesystem, reserve at
least 11 GiB free before accepting uploads. If they use separate filesystems,
reserve at least 7 GiB for media and 6 GiB for scratch. The API declines an
upload before either reserve would be exhausted. `MEDIA_MAX_DURATION` (default
`8h`) caps the length of one recording or upload.
`RECORDING_MAX_DURATION_HOURS` (default 8) caps one live recording session. If
it is longer than `MEDIA_MAX_DURATION`, the API logs a warning at startup and
limits recordings to `MEDIA_MAX_DURATION`.

## Install sequence

Start in a clean source checkout:

```bash
git clone https://github.com/nyata-ai/voxis-source-available.git
cd voxis-source-available/infra/oss
```

Copy `.env.example` to `.env`. Set the public hostname, URLs, CORS and MCP
origins, TLS email, the approved DCR hosts, the operator's Speechmatics
credential, and `BACKUP_AGE_RECIPIENT` (the public key of an age identity kept
off this host). Then run, in order:

```bash
bash scripts/bootstrap.sh
docker compose --env-file .env -f compose.yaml --profile app build
docker compose --env-file .env -f compose.yaml up -d --no-build --wait --wait-timeout 240 keycloak openbao
bash scripts/init-openbao.sh
bash scripts/configure-keycloak-master.sh
bash scripts/configure-keycloak-dcr.sh
bash scripts/configure-keycloak-mcp-audiences.sh
docker compose --env-file .env -f compose.yaml --profile app up -d --no-build --wait --wait-timeout 600
```

`bootstrap.sh` generates every secret marked `CHANGE_ME`, refuses to continue
while any placeholder remains, and makes the local directories. It never
initializes OpenBao.

`init-openbao.sh` initializes OpenBao with five unseal shares, any three of
which unseal it. It writes the shares, and nothing else, to
`secrets/openbao/operator-init.json`. It sets up the transit engine, the
application AppRole (in `.env`), and the backup AppRole (in
`secrets/openbao/backup-approle.json`). The initial root token is used only in
memory and revoked before the script exits, even if it fails. If it fails,
stop OpenBao, empty `OPENBAO_DATA_DIR`, delete `operator-init.json`, and run it
again.

The three `configure-keycloak-*.sh` scripts ask for a Keycloak administrator of
the `master` realm. The first time, use the temporary administrator from
`.env` (`KEYCLOAK_BOOTSTRAP_ADMIN_USERNAME` and `KEYCLOAK_BOOTSTRAP_ADMIN_PASSWORD`)
and leave the one-time code empty. Later, use your permanent administrator and
its current one-time code.

Run these scripts as the installation operator. A normal operator needs Docker
access and `sudo` for the narrow media operations below; a root-operated host
runs them directly. Bootstrap gives only media and scratch to API user 10001.
Backup and restore use that privilege only to read or restore API-owned media
and to prepare the empty OpenBao Raft directory for the image-owned account.
`.env`, the OpenBao files under `secrets/`, and media remain private; the
rendered Keycloak realm is public configuration and is readable by Keycloak.
It holds no secret: Keycloak reads the lookup-client secret from its
environment when it imports the realm.

## Unseal shares and custodians

Right after `init-openbao.sh`, give each of the five shares in
`operator-init.json` to a different trusted person (a custodian). Each stores
their share offline, for example on paper in a safe. Then delete the host copy:

```bash
shred -u secrets/openbao/operator-init.json
```

Nothing on the host can unseal OpenBao or act as root after that. Three
custodians are needed to unseal OpenBao after any restart, to restore a backup,
and to rotate the application AppRole. Backups never contain the shares.

OpenBao starts sealed after every restart. Start the core services, then have
three custodians each enter a share. The share is read without echo, so it
stays out of shell history:

```bash
docker compose --env-file .env -f compose.yaml up -d --no-build
read -rs -p 'OpenBao unseal share: ' SHARE; echo
printf '%s' "$SHARE" | docker compose --env-file .env -f compose.yaml exec -T openbao bao write sys/unseal key=-
unset SHARE
```

Repeat the last three lines for each custodian, then start the app and wait for
its health checks:

```bash
docker compose --env-file .env -f compose.yaml --profile app up -d --no-build --wait --wait-timeout 300
```

When a root token is needed (restore and AppRole rotation), the scripts ask the
custodians for their shares, make a temporary root token, and revoke it before
they exit. The unauthenticated root-generation API is open only on a loopback
listener inside the OpenBao container, reachable through `docker compose exec`.

## Keycloak administration

The public proxy serves only the `voxis-oss` realm and the login pages. The
admin console, the admin API, and the `master` realm answer 404 there. The
console is published only on the host's loopback address, at port
`OSS_KEYCLOAK_ADMIN_PORT` (default 8081). Reach it through an SSH tunnel from
your workstation, using the same port on both ends:

```bash
ssh -L 8081:127.0.0.1:8081 operator@your-host
```

Then open `http://localhost:8081/auth/admin/` in your browser. Use `localhost`,
not `127.0.0.1`: Keycloak serves the console only at that address.
`configure-keycloak-master.sh` points the `master` realm's sign-in pages at
this address, turns on brute-force protection, a password policy, and 90-day
login and admin event logs, and requires every new administrator to set up a
one-time password.

Replace the temporary administrator before admitting users:

1. Sign in to the console with the temporary administrator from `.env`.
2. In the `master` realm, create a named administrator, set a password of at
   least 12 characters, and assign the realm role `admin`.
3. Sign out and sign in as the new administrator. Keycloak asks you to set up
   a one-time password in an authenticator app.
4. Delete the temporary administrator under Users in the `master` realm.

Keycloak does not recreate the temporary administrator after this. The
`configure-keycloak-*.sh` scripts then accept the named administrator and its
one-time code.

## Users and roles

The realm disables self-registration, email delivery, and automatic email
verification. The operator must create users and mark each email verified only
after local identity verification. The API, REST administration, and MCP
middleware continue to require an `email_verified` token claim. Do not relax
that check to work around an incomplete initial account. Bootstrap renders and
imports only `realm-voxis-oss.json`; it never mounts the placeholder template.

In the console, select the `voxis-oss` realm and create each user with a
temporary password. This realm uses the email address as the username.
Passwords need at least 12 characters and may not reuse the last three.

- Give ordinary users the role `voxis-user`.
- Give operators the role `voxis-admin`. It includes `voxis-user`, so an admin
  can also use the application normally.

Mark each email verified only after you have verified it outside Keycloak. Each
user changes the temporary password and sets up a one-time password at first
sign-in. Test that flow with a synthetic user before admitting people.

Clients registered through MCP dynamic registration receive `voxis-user` in
their tokens, never `voxis-admin`.

`configure-keycloak-dcr.sh` updates only Keycloak's existing anonymous client
registration policies after import. Set `KEYCLOAK_DCR_TRUSTED_HOSTS` to the
exact approved client hosts first. The script allows Voxis MCP scopes plus the
identity and Keycloak baseline scopes that dynamic registration requires. It
does not change the authenticated registration policy.

Keycloak checks two things against `KEYCLOAK_DCR_TRUSTED_HOSTS`: the address
that sends the registration request, and the host of every redirect URI. The
redirect-URI hosts are compared as written. The sender is compared by IP
address, and Keycloak runs on internal networks that cannot resolve public
names, so a host-name entry never matches a sender. For each approved client,
list:

- the IP address its registration requests come from (its egress address, as
  your proxy sees it), and
- the host names in its redirect URIs.

A host name alone does not authorize a sender: registration fails with
`Host not trusted`. Do not give Keycloak internet access to work around this.

MCP needs the `email` and `email_verified` claims. New clients get the `basic`
and `email` scopes by default, and `basic` also carries both claims, because a
client that lists its own scopes at registration keeps only `basic` as a
default scope. An MCP client therefore works whether or not it asks for
`email`.

`configure-keycloak-mcp-audiences.sh` makes the retained MCP scope audience
mappers match the rendered realm template. Use it after upgrading an existing
realm because Keycloak skips a realm that already exists during
`--import-realm`. It changes only the named audience mappers in the eleven
retained MCP scopes. It leaves users, roles, clients, other scopes, and
sessions alone, and stops if it finds an unexpected combined mapper.

The API looks up the owner of an API key through the service-account client
`voxis-oss-api-lookup`, which may only view users. Bootstrap generates its
secret (`KEYCLOAK_USER_LOOKUP_CLIENT_SECRET`) and the realm import creates the
client with it.

## When someone leaves

- An administrator or user: disable or delete their Keycloak account, then sign
  out their sessions (Users, then Sessions, in the console).
- A custodian: their share still works until OpenBao is rekeyed. Rekeying is
  not scripted yet; plan it with the remaining custodians.
- Anyone who had host access: rotate the application AppRole (below) and
  replace the secrets in `.env` they could read.

## Network layout

Each Compose network joins only the services that must talk to each other:

| Network | Members | Internet |
| --- | --- | --- |
| `edge` | proxy, api | yes (TLS certificates, Speechmatics) |
| `web` | proxy, web | no |
| `identity-proxy` | proxy, keycloak | no |
| `identity` | api, keycloak | no |
| `identity-db` | keycloak, keycloak-db | no |
| `app-db` | api, postgres | no |
| `secrets` | api, openbao | no |
| `model` | api, gemma | no |
| `scan` | api, clamav | no |
| `scan-updates` | freshclam | yes (signature updates) |
| `model-download` | gemma-download | yes (one-time model fetch) |

PostgreSQL, OpenBao, the model runtime, ClamAV, and both databases are
reachable only by the service that uses them. The clamd scanner, which receives
uploaded audio, has no internet access. Signature updates run in the separate
`freshclam` service, which shares only the signature volume with it and never
sees an upload. clamd starts once the volume holds a full signature set. A new
volume starts with the signatures bundled in the ClamAV image, which can be
some weeks old; `freshclam` brings them up to date within minutes, and clamd
loads newer signatures within 10 minutes. Backup and restore reach the
databases with `docker compose exec`.

The `edge` range, `172.30.0.0/24`, is the API's `TRUSTED_PROXIES` value; do not
widen it. Keycloak trusts forwarding headers only from `identity-proxy`
(`172.30.1.0/24`). The application AppRole works only from `secrets`
(`172.30.2.0/24`), and the backup AppRole only from inside the OpenBao
container. These ranges are fixed in `compose.yaml`; pick other ranges there
and in `scripts/lib/openbao.sh` only if they collide with host networks.

If Compose fails with `Pool overlaps with other one on this address space`, or
the host loses a route after the stack starts, one of these ranges overlaps a
host, VPN, or other Docker network. Check with `ip route` and
`docker network inspect`. To move the stack, change all of these together:

| Range | Where it is set |
| --- | --- |
| `edge`, `172.30.0.0/24` | `compose.yaml` (`networks.edge`); `TRUSTED_PROXIES` in `.env`; the admin listener's `remote_ip 172.30.0.1` (the `edge` gateway) in `caddy/Caddyfile` and `caddy/Caddyfile.smoke` |
| `identity-proxy`, `172.30.1.0/24` | `compose.yaml` (`networks.identity-proxy`, and Keycloak's `--proxy-trusted-addresses`) |
| `secrets`, `172.30.2.0/24` | `compose.yaml` (`networks.secrets`); `openbao_app_cidr` in `scripts/lib/openbao.sh` |

The application AppRole's CIDR binding is written when `init-openbao.sh` runs,
so move `secrets` before installing. Changing `compose.yaml`, the Caddyfile,
or `scripts/lib/openbao.sh` also fails the attack-surface checks in
`scripts/verify-*-attack-surface.sh` until the change is reviewed.

Keycloak's discovery endpoint must pass its health check before Compose starts
the API, and the proxy waits for the API readiness check. The public Keycloak
base address is `OSS_KEYCLOAK_PUBLIC_URL`. `OSS_KEYCLOAK_FETCH_URL` is the
backend's private discovery address. Keycloak keeps the issuer public, but
returns the public keys used to verify login tokens on the private address for
backend callers. Do not replace the public base address with the container
address.

## Container hardening

Every service runs with `no-new-privileges`, no Linux capabilities except those
listed below, a process limit, rotated logs (5 files of 10 MB each), and
restarts after a crash or host reboot. Most also run with a read-only root
filesystem:

| Service | User | Added capabilities | Read-only root |
| --- | --- | --- | --- |
| postgres, keycloak-db | 70 (postgres) | none | yes |
| keycloak | 1000 | none | no: Keycloak builds its runtime at start |
| openbao | 100 (openbao) | none | yes |
| clamav, freshclam | root, then clamav | CHOWN, DAC_OVERRIDE, FOWNER, SETUID, SETGID | yes |
| gemma | 10001 | none | yes |
| gemma-download | root | CHOWN | yes |
| api | 10001 | none | yes |
| web | 10001 | none | yes |
| proxy | 10001 | NET_BIND_SERVICE | yes |

The ClamAV image prepares its directories as root before it drops to its own
account for `freshclam` and `clamd`, so both services keep those five
capabilities.

The API media sandbox uses the derived Docker seccomp profile and the named
`voxis-oss-api-bwrap` AppArmor profile. Bootstrap installs the AppArmor profile
in enforce mode and fails if it cannot do so. The AppArmor policy is a
mechanical Docker-default derivative that permits Bubblewrap mount and
pivot-root mediation only for the API container; it does not modify
`docker-default` or affect other containers. Compose still runs the API as UID
10001 with a read-only root filesystem, `no-new-privileges`, and no Linux
capabilities. The media scratch bind mount must be protected by the operator.
Compose does not claim that the host mount has `noexec`, `nosuid`, or `nodev`.
See `security/README.md` for the policy hashes and tradeoff.

The web pages carry a Content Security Policy that allows scripts, styles,
images, media, and connections only from the site itself. The site also sends
HSTS for one year, `nosniff`, a strict referrer policy, a permissions policy
that allows the microphone only for the site, and no server banner. Keycloak
and the API keep their own security headers.

## OpenBao storage and audit log

A fresh `OPENBAO_DATA_DIR` is prepared as UID:GID `100:1000` with mode `0700`:
those are the OpenBao image's own account and group. On the host they may show
up as unrelated names or bare numbers; that is expected, do not change them.
Bootstrap refuses to change retained Raft data with any other owner or mode.
Only the reviewed configuration file is mounted, and there is no web UI.

OpenBao writes an audit record of every request to
`OPENBAO_DATA_DIR/audit.log`, with secret values HMAC-hashed. It grows with
API traffic. If OpenBao cannot write it, OpenBao refuses requests and the API
stops working, so watch the free space and rotate the file: move it aside,
then run `docker compose --env-file .env -f compose.yaml kill -s HUP openbao`
so OpenBao reopens a new file. The audit log is not part of backups.

## AppRole secret rotation

Rotate the application AppRole secret ID when someone who could read `.env`
leaves, or on your own schedule. Three custodians must be present:

```bash
bash scripts/rotate-approle-secret.sh
```

The script asks for three unseal shares, makes a temporary root token, writes a
new secret ID to `.env`, recreates the API, and waits until it is ready. Only
then does it destroy the previous secret IDs and revoke the temporary root
token. If the API does not become ready, the old secret ID stays valid and the
script stops so you can investigate. A file with one share per line can replace
the prompts: `bash scripts/rotate-approle-secret.sh /path/to/shares`. Delete
such a file right after use.

The secret ID does not expire by itself, because no automation rotates it.

## Backups and recovery

Set `BACKUP_AGE_RECIPIENT` in `.env` first. Then run:

```bash
bash scripts/backup.sh
BACKUP_AGE_IDENTITY=/secure/path/age-identity.txt \
  bash scripts/verify-backup.sh BACKUP_DIRECTORY
```

`backup.sh` signs in with the backup AppRole, which can only take a Raft
snapshot and only from inside the OpenBao container. It revokes that one-hour
token when it exits. It stops the API and Keycloak, writes one quiesced set of
both databases, encrypted media, and the OpenBao snapshot, then restores their
prior running state. Each part is encrypted to `BACKUP_AGE_RECIPIENT`.

Backups hold no unseal shares and no root token: a stolen backup cannot be
decrypted by OpenBao without three custodians. Backups are encrypted but not
signed, so keep them on write-once or off-site storage that an attacker on this
host cannot change.

Run a restore only on a new host after `bootstrap.sh` creates empty
directories. Three custodians must be present:

```bash
BACKUP_AGE_IDENTITY=/secure/path/age-identity.txt \
  VOXIS_RESTORE_CONFIRM=RESTORE_FRESH_TARGET \
  bash scripts/restore.sh BACKUP_DIRECTORY
```

The restore rejects existing containers, Compose volumes, nonempty data paths,
unsafe archive member types, bad checksums, and backups made with an earlier
pre-release candidate (see [Fresh installation required](#fresh-installation-required))
before extraction. It decrypts
the media archive once into a private temporary directory under `TMPDIR`, which
needs free space equal to the media archive. It restores the databases and the
OpenBao Raft state, then asks for three unseal shares (or reads them from a file
given as a second argument). It uses them to unseal OpenBao and to make a
temporary root token, issues new AppRole secret IDs for this host, destroys the
ones issued to the old host, and revokes the temporary token. It also gives the
restored Keycloak realm this host's lookup-client secret. A full restore drill,
including encrypted-media recovery and a Keycloak login, remains a release
requirement.

The restored `master` realm comes from the backup. The
`KEYCLOAK_BOOTSTRAP_ADMIN_USERNAME` and `KEYCLOAK_BOOTSTRAP_ADMIN_PASSWORD`
that `bootstrap.sh` wrote on this host therefore do not work. Use the source
installation's permanent administrator and its one-time code for the console
and the `configure-keycloak-*.sh` scripts.

### Retrying a failed restore

A restore that fails after it starts the target's services leaves the target
partly restored, and `restore.sh` says so. A second run refuses that target.
Reset it to a fresh target first, from this directory:

```bash
docker compose --env-file .env -f compose.yaml --profile app down -v
sudo find data/media data/scratch data/openbao -mindepth 1 -delete
rm -f secrets/openbao/backup-approle.json
bash scripts/bootstrap.sh
```

These are the default paths. If you changed them, empty your
`LOCAL_STORAGE_DIR`, `LOCAL_SCRATCH_DIR`, and `OPENBAO_DATA_DIR` instead, and
delete your `OPENBAO_BACKUP_APPROLE_FILE`. Keep the directories themselves.
Fix the cause of the failure, then run `restore.sh` again.

For a disposable acceptance stack, set `VOXIS_OSS_COMPOSE_OVERRIDE_FILES` to
colon-separated Compose override paths. Use absolute paths when calling the
scripts outside this directory. Keep the storage and recovery paths in `.env`
aligned with their mounted paths; use an override only for transport settings
or the Gemma endpoint.

## Model status

The pinned default is Google Gemma 4 12B QAT Q4_0. The local path selected by
`GEMMA_MODEL_FILE` points to `gemma-4-12b-it-qat-q4_0.gguf`. The bundled CPU
recipe stays at a 16,384-token context. A structured request can use at most
five roughly 4 KiB serialized source chunks, or about 20 KiB. A separate L4
test covered 32K capacity; it does not support changing the CPU default.

The CPU acceptance run observed one model call take almost five minutes. The
default request deadline is therefore 360 seconds, and the complete summary
job has a 60-minute ceiling for sequential structured calls. The configuration
accepts request deadlines from 1 to 600 seconds and job ceilings from 1 to 120
minutes; a job ceiling must be longer than one request. These are cancellation
bounds, not a throughput or long-transcript guarantee.

The pinned llama.cpp revision rejects the anchored schema when its `fact`
field has `maxLength: 2000`: its grammar repetition guard fails before
generation. The same schema passes at `maxLength: 1999`; the prompts use
that compatible bound and the full anchored fixture set passed with it. Keep
the application validation bounds in force. A future runtime change requires
the same schema probe and an anchored fixture run before raising this bound.

Read [model validation](../../docs/model-validation.md) for the exact weights,
runtime, synthetic 12B and 26B comparison, later 12B grounding and capacity
checks, and their limits. Those checks do not prove a customer-data flow or a
full application stack.

The runtime does not expose an inference port to the host. Only the API can
reach it, over the `model` network, and it has no internet access. The
configured context is a measured feasibility limit, not proof of
long-transcript support.
