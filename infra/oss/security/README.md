# API media sandbox policies

The API uses Bubblewrap for media tools. Compose keeps that container at UID
10001, read-only, without Linux capabilities, and with `no-new-privileges`.
It applies two derived Moby policies.

`bwrap-moby-v29.7.2-seccomp.json` adds the six nested-namespace system calls
Bubblewrap needs to the Docker Engine 29.7.2 seccomp default. Its provenance
records the source and derived hashes.

`bwrap-moby-v29.2.0-apparmor.profile` is a mechanical render of Docker Engine
29.2.0's AppArmor default with exactly two changes: it replaces `deny mount,`
with `mount,` and adds `pivot_root,`. Bubblewrap makes a nested user and mount
namespace for these operations. AppArmor 3.0.8 cannot express its dynamic bind
and remount sequence as a reliable narrower rule set. This profile preserves
the rest of the Moby default policy and applies only to the API container. It
does not change Docker's `docker-default` profile or affect other containers.

`bootstrap.sh` installs the named AppArmor profile in enforce mode. It fails if
AppArmor or its parser is unavailable, disabled, or cannot load that exact
reviewed profile. The scratch bind mount is operator-provisioned. Compose does
not claim or enforce `noexec`, `nosuid`, or `nodev` options for it.

Run `python3 render-bwrap-apparmor-profile.py` to verify the AppArmor profile
against the checked-in Moby template before updating it. Preserve the hashes in
both provenance files when changing Docker Engine or either derived policy.

The copied Moby policy is copyright The Moby Authors and licensed under Apache
2.0. `LICENSE.moby` contains the complete license. Preserve this attribution
and license text in `THIRD_PARTY_NOTICES.md` when preparing a release.

## Keycloak image dispositions

`keycloak-26.7.5.openvex.json` records the two reviewed HIGH or CRITICAL
Keycloak library matches from the runtime built from the locked Keycloak
distribution and Temurin 21 JRE images. Before scanning, the workflow binds
the statement to a package found in that runtime's SBOM. It fails if the
reviewed package disappears, changes, or a new HIGH or CRITICAL match appears.

The document does not apply to a changed runtime image or Keycloak deployment.
`verify-keycloak-attack-surface.sh` binds the review to the two pinned source
images, the runtime Dockerfile, and the exact Compose, realm, and Caddy inputs.
Review the VEX again whenever that guard fails, before changing its statements,
or when a vendor publishes a replacement advisory.

Trivy 0.74 reports two HIGH or CRITICAL results for the guarded runtime image:
mssql-jdbc for CVE-2025-59250, and Guava for CVE-2026-102554, covered below. Each
statement names only that exact Maven package PURL, so a different package or
finding fails the final scan. The scan reports no HIGH or CRITICAL
operating-system package result.

Keycloak 26.7.5 ships `org.freemarker:freemarker` 2.3.35, which fixes
CVE-2026-84939 in the 2.3.32 library that 26.7.4 bundled. It also ships
netty-handler 4.1.138.Final and bcprov-jdk18on 1.86, so the earlier statements
for CVE-2026-75595, CVE-2026-13506, and CVE-2026-8763 were removed rather than
carried forward. The 26.7.5 release also fixes Keycloak advisories including
CVE-2026-89298, CVE-2026-18206, and CVE-2026-18211; see the
[release notes](https://github.com/keycloak/keycloak/releases/tag/26.7.5).
The Temurin 21 JRE image is re-locked when its packages gain a finding. The
current digest clears `libbrotli1` CVE-2026-111497.

The upstream `26.7.5` source tag resolves to commit
`44299f39868f252ea02236134618c8c5c70a6e9d`. The runtime contains
`mssql-jdbc-13.2.1.jre11.jar`, which the
[Microsoft JDBC release notes](https://learn.microsoft.com/en-us/sql/connect/jdbc/release-notes-for-the-jdbc-driver?view=sql-server-ver17)
list as the fixed build for CVE-2025-59250. Trivy normalizes that package
version to 13.2.1 and still matches the advisory range.

### Guava CVE-2026-102554

Keycloak 26.7.5 ships Guava 33.5.0. The fix is in 33.7.2, and Keycloak 26.8.1
still ships an affected 33.6.0, so upgrading would not clear this match. The
advisory needs Java native deserialization of an attacker-supplied
`CompactHashMap`, `CompactHashSet`, or `MapMakerInternalMap`.

Among Keycloak's own jars, only Kerberos federation, the Twitter identity
provider, and two principal classes use `ObjectInputStream`. The reviewed realm
defines no federation or identity providers, and only the proxy and the API can
reach Keycloak, on internal Compose networks. This review did not trace every
one of the 50 bundled jars that use `ObjectInputStream`, so it is a judgement
about this deployment and not a proof. Replace the statement when Keycloak ships
Guava 33.7.2 or later.

## OpenBao image dispositions

`openbao-2.6.5.openvex.json` records the HIGH matches that Trivy 0.74 reports
for the locked OpenBao 2.6.5 image
(`quay.io/openbao/openbao@sha256:09f1eaaee351c329d75a5a89654d6e1ad4792998650c3cbf0c3513a76d9d9b6e`,
amd64 manifest `sha256:f97a61a1814dccc70d04d45d15fed1827442f82d3848c3579a5e951c979d045b`).

Five are on the OpenBao module itself, which Trivy names by the pseudo-version
`v0.0.0-20261026193202-b814361ea234` of release commit
`b814361ea23440b881b43882e9d1bd950342471c`. Each advisory was fixed in an
earlier release (CVE-2024-8185 and CVE-2024-9180 in 2.0.3, CVE-2025-59043 in
2.4.1, CVE-2025-64761 in 2.4.4, CVE-2026-45808 in 2.5.4), so 2.6.5 does not
contain the vulnerable code.

The sixth is CVE-2026-116587, on go-jose v3.0.5 and v4.1.4. Unlike the others,
this code is linked into `bao`, and OpenBao 2.7.2 also still ships these
versions. The advisory needs an attacker-supplied RSA JWK. The initialization
policy enables only the transit engine and AppRole auth, so no JWT or OIDC auth
method parses external JWKs. Replace this statement when OpenBao ships go-jose
4.1.6 or later. An operator who enables JWT or OIDC auth is outside this review.

Before each scan, `render-vex-for-image.py` binds every statement to the exact
package PURL found in the scanned runtime. A changed module version or a new
finding fails the scan. A rebuild refreshes Alpine packages and gets a new image
ID.

The runtime is hardened in Compose: UID 100, read-only root filesystem, no Linux
capabilities, and only the `secrets` network, which the API alone shares. The
configuration disables the web UI, declares a file audit device, and opens the
legacy unauthenticated root-generation API only on a loopback listener inside
the container.

`verify-openbao-attack-surface.sh` binds the review to the image, runtime
Dockerfile, Compose file, listener configuration, and initialization policy
(`scripts/init-openbao.sh` and `scripts/lib/openbao.sh`). Review every
statement if that guard fails.

## Caddy image dispositions

`caddy-2.11.7.openvex.json` records one HIGH match for the Caddy and web images:
CVE-2026-116587 on go-jose v3.0.5. That major version has no fixed release.
The build raises go-jose v4 to 4.1.7, so only v3 remains.

The code is linked into `caddy`, because the embedded Smallstep authority used
by `caddypki` depends on it. JWK parsing belongs to the ACME server and PKI app.
No reviewed configuration runs either, and the admin API is off. The security
job runs `verify-caddy-attack-surface.sh`, which fails if `acme_server` or `pki`
appears in a Caddy configuration, including the one the web image writes from its
Dockerfile, or if any of those inputs change. It cannot see a configuration an
operator adds at deployment. An operator who enables them is outside this review.

## Local model image dispositions

The 2 November 2026 review covers the 11 CVEs that Trivy 0.74 reports for the
packages this Dockerfile installs on Debian 13.7, measured from its runtime
stage. Two earlier Curl rows, CVE-2026-8286 and CVE-2026-8927, no longer match
now that the image installs Curl `8.14.1-2+deb13u7`. CVE-2026-119373 is new.
The attack-surface guard binds this review to the Dockerfile, Compose file,
model environment, and Gemma request builder.
Before each scan, `render-vex-for-image.py` replaces each statement's product
with the exact package PURLs found in that image, so the rendered document is
scoped by package and not by image. The workflow uses it only for the image it
was rendered from. A package-version change or a new CVE fails the scan.

The reviewed runtime uses a local model path and the private Compose `model`
network, which only the API shares and which has no internet access. It has
no host port or proxy environment. Compose runs it read-only as UID 10001 with
no Linux capabilities. Its fixed command has no Hugging Face or
model-URL option, and `--no-mmproj` excludes the multimodal projection. In
llama.cpp `5266f24da75dc449bd56cbed7addb9c8e4a6a73e`, remote model handling is
conditional on those command-line parameters in
`tools/server/server.cpp` (lines 121-132 and 386-400). Voxis sends only two
role-and-content text messages in
`backend/internal/adapter/gemma/client.go` (lines 349-370 and 531-543), so a
transcript cannot add an `image_url`, file, or model-download field.

The Curl rows require the missing options or protocols described in
[CVE-2026-12064](https://curl.se/docs/CVE-2026-12064.html) and
[CVE-2026-8458](https://curl.se/docs/CVE-2026-8458.html). CVE-2026-119373 needs
crafted SQLite FTS5 data: only `liblastlog2` depends on `libsqlite3` here,
`libcurl` and `libgomp` do not link it, and llama.cpp has no SQLite code at the
reviewed revision. The remaining rows
require `infocmp` [CVE-2025-69720](https://ubuntu.com/security/CVE-2025-69720),
the absent `systemd-homed` daemon, pathname-based libacl use
[CVE-2026-54369](https://linux.oracle.com/errata/ELSA-2026-42736.html), local
util-linux tools [CVE-2026-76642 and CVE-2026-78410](https://seclists.org/oss-sec/2026/q3/652),
[CVE-2026-78408](https://access.redhat.com/security/cve/cve-2026-78408),
[CVE-2026-78409](https://access.redhat.com/security/cve/cve-2026-78409), or Perl
`Archive::Tar` parsing [CVE-2026-9538](https://linux.oracle.com/cve/CVE-2026-9538.html).

An operator using a different command line, image, model downloader, network
setting, or any of those reviewed inputs is outside this evidence and must run
a new scan and review before use. The Dockerfile refreshes distribution packages
at build time, so a new build is not assumed to have the same image ID.
