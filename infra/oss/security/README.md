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

`keycloak-26.7.5.openvex.json` records the one reviewed HIGH or CRITICAL
Keycloak library match from the runtime built from the locked Keycloak
distribution and Temurin 21 JRE images. Before scanning, the workflow binds
the statement to a package found in that runtime's SBOM. It fails if the
reviewed package disappears, changes, or a new HIGH or CRITICAL match appears.

The document does not apply to a changed runtime image or Keycloak deployment.
`verify-keycloak-attack-surface.sh` binds the review to the two pinned source
images, the runtime Dockerfile, and the exact Compose, realm, and Caddy inputs.
Review the VEX again whenever that guard fails, before changing its statements,
or when a vendor publishes a replacement advisory.

Trivy 0.74 scanned the guarded runtime image
`sha256:a6250ea769b3acda9a52dacedb4c25445a4a68f1c6cef6c2035b63d65bfcd201`
without a VEX. It reported one HIGH or CRITICAL result: mssql-jdbc for
CVE-2025-59250. The statement names only that exact Maven package PURL, so a
different package or finding fails the final scan. The scan found no HIGH or
CRITICAL operating-system package result.

Keycloak 26.7.5 ships `org.freemarker:freemarker` 2.3.35, which fixes
CVE-2026-84939 in the 2.3.32 library that 26.7.4 bundled. It also ships
netty-handler 4.1.138.Final and bcprov-jdk18on 1.86, so the earlier statements
for CVE-2026-75595, CVE-2026-13506, and CVE-2026-8763 were removed rather than
carried forward. The 26.7.5 release also fixes Keycloak advisories including
CVE-2026-89298, CVE-2026-18206, and CVE-2026-18211; see the
[release notes](https://github.com/keycloak/keycloak/releases/tag/26.7.5).
The Temurin 21 JRE image was re-locked to a current build because the earlier
digest carried OpenSSL 3.0.13-0ubuntu3.15 (CVE-2026-84782, HIGH); the locked
image has 3.0.13-0ubuntu3.16.

The upstream `26.7.5` source tag resolves to commit
`44299f39868f252ea02236134618c8c5c70a6e9d`. The runtime contains
`mssql-jdbc-13.2.1.jre11.jar`, which the
[Microsoft JDBC release notes](https://learn.microsoft.com/en-us/sql/connect/jdbc/release-notes-for-the-jdbc-driver?view=sql-server-ver17)
list as the fixed build for CVE-2025-59250. Trivy normalizes that package
version to 13.2.1 and still matches the advisory range.

## OpenBao image dispositions

`openbao-2.6.3.openvex.json` records the five HIGH matches that Trivy 0.74
reports for the runtime built from the locked OpenBao 2.6.3 image
(`quay.io/openbao/openbao@sha256:a60afafda36337abe833c4a63894bf1095098f29abea4091e7e555a33dd52889`,
amd64 manifest `sha256:99c8dd178200d9a5f1a0420d6b1923514280e31e9271bdd92c69f765738c42aa`).
All five are on the OpenBao module itself, which Trivy names by the pseudo-version
`v0.0.0-20260923163753-63a65e6b9075` of release commit
`63a65e6b907589dbb952c371a70260a065bf8bd7`. Each advisory was fixed in an
earlier release (CVE-2024-8185 and CVE-2024-9180 in 2.0.3, CVE-2025-59043 in
2.4.1, CVE-2025-64761 in 2.4.4, CVE-2026-45808 in 2.5.4), so 2.6.3 does not
contain the vulnerable code. The scan found no CRITICAL row. The Go dependency
findings that 2.6.2 carried (go-archive, x/crypto SSH, and two gRPC advisories)
are gone: 2.6.3 ships the fixed module versions.

2.6.3 fixes GHSA-j6wc-jpvg-xfxq (plugin command path), GHSA-cg72-x35g-xfp8,
GHSA-hr5j-3j78-4vh2, GHSA-mjch-vcw3-hhmf, and further advisories listed in the
[2.6.3 release notes](https://github.com/openbao/openbao/releases/tag/v2.6.3).

Before each scan, `render-vex-for-image.py` binds every statement to the exact
package PURL found in the scanned runtime. A changed module version or a new
finding fails the scan. The derived runtime measured for this review was
`sha256:a91ffcb023fc9cfb5a5d21d71fbdf71315e717a2f54a0dce047a982e54b276be`; a
rebuild refreshes Alpine packages and gets a new image ID.

The runtime is hardened in Compose: UID 100, read-only root filesystem, no Linux
capabilities, and only the `secrets` network, which the API alone shares. The
configuration disables the web UI, declares a file audit device, and opens the
legacy unauthenticated root-generation API only on a loopback listener inside
the container.

`verify-openbao-attack-surface.sh` binds the review to the image, runtime
Dockerfile, Compose file, listener configuration, and initialization policy
(`scripts/init-openbao.sh` and `scripts/lib/openbao.sh`). Review every
statement if that guard fails.

## Local model image dispositions

The release review covers 51 raw HIGH or CRITICAL matches across 12 CVEs from
derived model image ID
`sha256:21389eaa2d93f68c5d25f406be81f9700d8cb2124769bb4ab2bded07ebefd743`.
The raw report remains separate; its SHA-256 is
`d0d96adad9fd77dde4d360e22299e5d9921fe2990a2e882850c3a7c8f9c0ad1b`.
This evidence applies to Dockerfile SHA-256
`2ded27b51d165853a07877b3241904b7bc27522662d75f2cd434ffde6053818f`,
Compose SHA-256
`f17cbf02ea783b5d92596a8160661a2e956aec6f39268c0ff028b4176a1e98a9`,
model environment SHA-256
`16a29ecc93d9ba599f3e302f008e665f8545574fa643cd9a40f76369d0f8b95e`,
and Gemma request builder SHA-256
`2c0ed26ef67cc9c828659091444eab802a138c3a57c9612965e8ed8bfc58c941`.
Before each scan, `render-vex-for-image.py` replaces only the VEX product with
Trivy's OCI package identifier for that image. The reviewed package PURLs stay
exact, so a package-version change or a new CVE fails the scan.

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
[CVE-2026-12064](https://curl.se/docs/CVE-2026-12064.html),
[CVE-2026-8286](https://curl.se/docs/CVE-2026-8286.html),
[CVE-2026-8458](https://curl.se/docs/CVE-2026-8458.html), and
[CVE-2026-8927](https://curl.se/docs/CVE-2026-8927.html). The remaining rows
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
