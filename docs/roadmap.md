# Roadmap

Voxis Source-Available is at an early stage. This page describes what the first release contains and the gaps we know about. It is not a promise of features or dates.

## What the first release contains

A Linux Docker Compose installation with:

- the web app and API;
- PostgreSQL, a local Keycloak for sign-in, and OpenBao for encryption keys;
- ClamAV malware scanning and a media-parsing sandbox;
- encrypted local storage for audio, transcripts, and summaries;
- a local Gemma 4 12B model for summaries; and
- transcription through the operator's own Speechmatics cloud account.

It supports uploads, browser recording with recovery, the library and reader, editing, search, summaries, exports, activity tracking, API keys, MCP, settings, and administration.

It leaves out URL transcription, privileged recording, billing, payments, credit purchases, Gemini provider controls, self-service sign-up, social login, Turnstile, and contact-email workflows. These are not planned for this edition.

## Known gaps

These are areas where the software is limited today. We may address some of them; none has a date.

- **Signed releases.** Release images are not signed yet.
- **Record binding for encrypted fields.** Encrypted values are not yet bound to the ID of the record they belong to.
- **Signed backups.** Backups are encrypted but not signed.
- **Storage quotas.** Not enforced in this edition.
- **On-premises transcription.** Speechmatics offers on-premises deployments by separate arrangement. Voxis has not been verified end to end with one.
- **Larger summary context on CPU.** The bundled CPU setup uses a 16K context. A 32K context has been tested only on a GPU.
- **Summary evidence.** Timestamp and speaker grounding, anchored summaries across several chunks, and speed on typical hardware have not been measured. See [model validation](model-validation.md).

For current limits, see [capability limits](capability-limits.md) and [data handling](data-handling.md). For what has been tested, see [acceptance evidence](acceptance-evidence.md).
