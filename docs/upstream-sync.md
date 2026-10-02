# Upstream sync

Voxis Source-Available is built from code shared with Nyata's commercial Voxis source, plus files that belong only to this edition. Each export includes `UPSTREAM.json`. It records the source revision, the edition revision, the exporter and manifest hashes, and a SHA-256 digest for every delivered file.

For a file marked as shared, its digest shows that the delivered bytes match the reviewed source revision. Files that set up the local AI provider, identity, storage, deployment, and other choices for this edition are edition files. Their digests identify the delivered version, but they do not claim that another Voxis deployment uses the same settings.

The pre-release security hardening changed some previously shared files in this repository. Those files are recorded as edition files until the commercial source carries the same changes.

## How updates are made

An update starts from a committed source revision and an empty staging area. The exporter copies only files named in the versioned allowlist. It rejects new source paths until they are classified, rejects unsafe paths and symbolic links, and checks the staged result for common secret and deployment material. An export that includes executable files must be applied and committed from a POSIX checkout, because Windows cannot record Git executable bits reliably.

Each update arrives as a pull request. Maintainers review every source commit and changed file since the previous reviewed revision, with attention to security, dependency, schema, query, test, and fixture changes. A fix for a security problem may be published after a delay, once it is safe to release.

This repository keeps its own history, release notes, policy files, and contributor work. Before an export writes anything, it compares every previously exported file with its recorded digest. A changed or unexpected file stops the update until a maintainer reconciles it.

The sync is started by hand and runs one at a time. It uses a credential limited to this repository's contents and pull requests. If that credential is not configured, the job stops before it changes anything. This repository's own continuous-integration jobs do not receive the sync credential.

## Contributing shared changes

Open an issue or pull request here. Maintainers will decide whether the change belongs in the shared source, where a later export carries it into this edition, or only in this edition's files.

`UPSTREAM.json` is provenance for this source tree. It does not describe a live deployment, its configuration, or its runtime behaviour.
