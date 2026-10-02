# Capability limits

This page lists the limits that matter when you use or run Voxis Source-Available. The tests behind these statements are described in [acceptance evidence](acceptance-evidence.md).

## Audio and uploads

- Audio longer than `MEDIA_MAX_DURATION` (default 8 hours) is refused.
- A live recording stops at `RECORDING_MAX_DURATION_HOURS` (default 8 hours). If that is set longer than `MEDIA_MAX_DURATION`, recordings are limited to `MEDIA_MAX_DURATION` and the API logs a warning at startup.
- The default configuration accepts four uploads at a time, each up to 1 GiB. The API refuses an upload before the reserved free disk space would run out. The [installation guide](../infra/oss/README.md) gives the disk reserves.
- Uploaded media is parsed inside a sandbox. An operator can set `MEDIA_SANDBOX=disabled`, but this removes an important protection and is not recommended.
- Storage quotas are not enforced in this edition. Watch disk space on the host.

## Request limits

These limits keep one person or one address from using up the server. They are fixed in this edition.

- **Uploads:** each user can have two uploads in progress at a time, within the server-wide limit above. A third is refused (HTTP 429) with a message asking the user to wait.
- **Starting transcriptions:** each user can start 30 transcriptions per hour, with bursts of up to 10 at once. The app, the REST API, and the MCP tools share this allowance.
- **Requests from one address:** before sign-in is checked, the API accepts up to 1,200 requests per minute from one client IP address, on `/api/*` and `/mcp`. Users behind one office network share this allowance.
- **Summaries and other model work:** the local model handles two requests at a time. A request that a person is waiting on waits up to 30 seconds for a free slot, then fails with a "model busy" error and can be retried. Background summary jobs wait for a slot without that time limit.

## Transcription

Voxis Source-Available uses Speechmatics Melia 1 for batch transcription through Speechmatics' cloud API. Speechmatics' feature list shows custom dictionary support for Melia 1 as "Not yet". Voxis therefore does not send vocabulary terms, and setting `CUSTOM_VOCABULARY_ENABLED=true` stops startup with a clear error. The app, the API, and MCP all report custom vocabulary as unavailable. This is a limit of the provider model, not a setting an operator can switch on. (Source checked 2026-09-13: <https://www.speechmatics.com/product/features-and-deployments>.)

Recognition accuracy, speaker labelling quality, and speed depend on Speechmatics and on the recording. This project has not measured them.

## Summaries

The bundled setup runs Gemma 4 12B on CPU with a 16,384-token context.

- A summary can use up to five chunks of about 4 KiB of transcript text each, so about 20 KiB of transcript in total. Longer input fails with a clear error before the model is called. It is never silently cut short.
- A runtime with a context of 32,768 tokens or more can accept up to 16 KiB per chunk, or about 80 KiB in total. Only use that setting after testing it on your own hardware; the bundled CPU setup has not been measured at 32K.
- Each model request is cancelled after 360 seconds, and a whole summary job after 60 minutes. These are safety limits, not speed promises. On CPU, a summary of a long transcript can take many minutes.
- High-stakes summary profiles are off by default.
- The summary endpoint must stay on a private network. The app warns if it has a public address.

Not yet established: meeting-quality scoring, speed on typical hardware, timestamp or speaker grounding of citations, and anchored summaries across several chunks. See [model validation](model-validation.md).

## Export

Export is available as PDF, DOCX, and JSON. PDF export does not reliably support Chinese, Japanese, or Korean scripts; use DOCX or JSON for those.

## Not included

URL transcription, privileged recording, billing and credits, self-service sign-up, social login, and email workflows are not part of this edition.
