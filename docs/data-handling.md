# Data handling and trust boundaries

This document explains what Voxis Source-Available stores, where data goes, how deletion works, and which limits remain. It is written for operators, data-protection officers, and auditors. It describes the software's behaviour; each operator is still responsible for how an installation is run.

## What is stored

| Data | How it is stored |
| --- | --- |
| Audio, transcripts, and summaries | Encrypted by the application with a per-organization key before they reach disk or the database. |
| Media details: filename, title, description, type, size, duration, hash, timestamps | Ordinary database columns, not encrypted. Collection names and descriptions are also ordinary data. |
| Accounts, jobs, errors, model metadata | Ordinary database data. Voxis does not encrypt the whole database. |
| Organization keys | Managed by the bundled OpenBao key store. The application loads organization keys into its own memory while it runs. |
| Scratch files | While a file is checked, scanned, converted, or sent to Speechmatics, a plaintext copy can exist briefly in protected scratch storage on the server. Scratch storage is not part of the encrypted-storage or backup design. |
| Logs | Record IDs, status values, and error categories. They do not record filenames, titles, descriptions, or transcript text. Raw error bodies from HTTP requests and providers are left out. Treat logs as sensitive all the same. |

## Where data goes

**Speechmatics (cloud transcription).** The server sends the audio to Speechmatics using the operator's own Speechmatics account. It sends a generic filename, not the original one, and tries to strip embedded media metadata first. If stripping fails, it sends the original file, so embedded metadata can reach Speechmatics. Speechmatics returns the transcript. After a job finishes, Voxis asks Speechmatics to delete it. A periodic cleaner retries any deletion that failed.

The default endpoint is Speechmatics' EU region, `https://eu1.asr.api.speechmatics.com` (`SPEECHMATICS_API_URL` in `infra/oss/.env.example`). Operators can choose another Speechmatics region. The operator is responsible for the Speechmatics account, its charges, its terms, and any data-processing agreement. Speechmatics also offers on-premises deployments by separate arrangement; Voxis Source-Available has not been verified with one.

**Local Gemma model (summaries and questions).** Summaries, key points, action items, and answers to questions are produced by the Gemma model that the operator runs, normally on the same host. Transcript text is sent only to that endpoint. The endpoint must stay on a private network. The application warns if it is configured with a public address. High-stakes summary profiles are off by default.

**Other network use.** The installation contacts container registries for images, model sources for model files, ClamAV for malware signature updates, and, if Caddy obtains a public certificate, a certificate authority. These services can see the server's network address and request details. They do not receive user content.

## Browser recording backup

While recording in the browser, Voxis keeps pieces of audio that have not been uploaded yet in the browser's local storage (IndexedDB). This lets a recording recover after a closed tab or a network drop.

- The stored pieces are encrypted, but the key is stored in the same browser. This protects only against casual inspection, not against someone with access to the device or browser profile.
- The store is cleared when the user signs out. If some pieces have not been uploaded, the user is warned before signing out.
- The store is also cleared when a different user signs in on the same browser.

On shared computers, users should finish uploading and sign out.

## Access and offboarding

Voxis uses the bundled Keycloak realm `voxis-oss`, with web client `voxis-oss-web` and API client `voxis-oss-api`. Browser sign-in uses PKCE (S256) and needs HTTPS or localhost.

- A user needs the Keycloak realm role `voxis-user` to use the app. Without it, the user sees a message asking them to contact their administrator. Administrators also need `voxis-admin`.
- Passwords and multifactor authentication are managed in the Keycloak Account Console.
- Sign-in tokens last up to 300 seconds. Disabling a user in Keycloak does not cancel a token already issued before it expires.
- API keys are separate credentials with their own expiry. They stop working within about 60 seconds after the user is disabled in Keycloak or loses the `voxis-user` role. Revoking a leaving user's keys explicitly is still good practice.
- The Keycloak admin console is not exposed publicly. Administrators reach it on the host's loopback interface over an SSH tunnel.

## Deletion

Deleting a recording or media file permanently removes, from the server:

- its audio;
- its transcripts and summaries; and
- its filename, title, and description.

This is permanent removal, not a hidden "deleted" flag. Existing backups keep their copies until those backups are deleted or rotated out, so the operator's backup retention decides when deleted data finally disappears.

Deleting a transcript permanently removes it and its summaries in the same way. Deleting a single summary through the API only hides it, because regenerating that summary type restores it; its text is removed when the transcript or media is deleted.

Recording retention, when an administrator turns it on, deletes the audio of browser recordings after the chosen period, using the same permanent removal. The transcripts and summaries of those recordings stay until someone deletes the recording.

### Removing all data for one person

An operator can remove everything that belongs to one user with a command run in the API container. Without `--confirm`, it only reports what it would delete.

**First disable the account in Keycloak**, then wait five minutes (the sign-in token lifetime) before you run the purge with `--confirm`. The command deletes the person's API keys before anything else, but it cannot sign them out. If they are still signed in, a file they upload while the purge runs could be left behind.

```bash
docker compose --env-file .env -f compose.yaml exec api voxis-api purge-user --subject <keycloak-user-id>
docker compose --env-file .env -f compose.yaml exec api voxis-api purge-user --subject <keycloak-user-id> --confirm
```

The Keycloak user ID is shown on the user's page in the Keycloak admin console. The command refuses a workspace that has more than one member. If Speechmatics still has jobs waiting to be deleted, it keeps an empty workspace record and asks you to run it again later.

After the purge you can delete the account in Keycloak. Backups made before the purge still hold the person's data until they rotate out. Each backup also contains the OpenBao key store, including the key that decrypts that data, so deleting the key from the running OpenBao would not make those copies unreadable. Shorten backup retention, or delete the affected backups, if the copies must go sooner.

### Giving a person a copy of their data

A user can download each audio file, and export each transcript and summary as PDF, DOCX, or JSON, from the app. For many items, the REST API (with an API key) and the MCP tools can list and export the same content. An operator handling a data request can help the user run these exports. There is no single one-click export of everything.

## Backups and keys

- Backups are encrypted with `age`. They are not signed, so keep them off the server and preferably on write-once storage.
- Backups contain encrypted content and ordinary database data. They keep deleted data until they are rotated out.
- The OpenBao root token is revoked after setup. OpenBao unseal key shares should be held by named custodians away from the server. They are not included in backups. If too many shares are lost, the encrypted data cannot be recovered.
- Test a restore regularly. The [installation guide](../infra/oss/README.md) describes the procedure.

## Network and container layout

Containers are split into separate networks. The databases, the OpenBao key store, the Gemma model runtime, and the ClamAV scanner that receives uploads have no internet access. ClamAV signature updates run in a separate container that never receives an upload. Uploaded media is parsed inside a sandbox unless the operator sets `MEDIA_SANDBOX=disabled`, which we do not recommend. The web app sends security headers, including a Content Security Policy.

## Known limits

- **Host administrators are inside the trust boundary.** The application loads organization keys into its memory. Anyone who controls the host, the running containers, or the key store can read the data.
- **Encrypted fields are not bound to their records.** Encrypted values do not carry the ID of the record they belong to (no additional authenticated data). Someone with write access to the database could move an encrypted value from one record to another within the same organization, and the application would decrypt it without noticing.
- **Speechmatics is a cloud processor.** It receives the audio. Its handling is governed by the operator's agreement with Speechmatics.
- **Metadata is not encrypted.** See the table above.
- **Release images are not signed.** Build from source or verify image digests.
- **Storage quotas are not enforced** in this edition. Monitor disk space on the host.

## Operator checklist

Before admitting users, record:

- the Speechmatics account, region, and data-processing terms;
- the model file, runtime, and revision in use;
- where media, scratch files, and backups are stored, and how scratch space is cleaned;
- who holds the OpenBao unseal shares;
- backup retention, and the date of the last restore test;
- recording retention settings;
- who can read logs; and
- the offboarding steps: disable the user in Keycloak, remove the `voxis-user` role, revoke API keys, and purge data if required (disable the account before the purge).

See also [capability limits](capability-limits.md), [model validation](model-validation.md), and [third-party notices](../THIRD_PARTY_NOTICES.md).
