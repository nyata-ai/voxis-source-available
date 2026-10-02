# Voxis Source-Available

Voxis Source-Available is a self-hosted web app for turning audio into searchable transcripts and AI summaries. You run it on your own Linux server. Speechmatics, a cloud service, does the transcription. A Gemma model running on your own hardware writes the summaries.

It is **source-available, not open source**. You can read, run, and change the code, but the [license](LICENSE) limits who may use it for free and for what. It is not an Open Source Initiative (OSI) approved license.

It is made by PT. Karya Nyata Teknologi (Nyata.AI). Nyata also runs Voxis as a hosted service. This edition is for people who want to run it themselves.

## Who it is for

- Schools, universities, charities, community groups, and religious organizations that want to transcribe meetings, lessons, or talks on their own server.
- Individuals using it for personal, noncommercial purposes.
- Businesses and auditors who want to inspect or test the code before deciding on a commercial license.

You will need someone comfortable running Docker on a Linux server. The [installation guide](infra/oss/README.md) walks through every step.

## What it does

- Upload audio files, or record in the browser with recovery if the page closes or the network drops.
- Transcribe with speaker labels, then read, search, and edit transcripts.
- Create summaries, key points, action items, and question-and-answer lists with a local AI model.
- Export transcripts and summaries as PDF, DOCX, or JSON.
- Use API keys and a Model Context Protocol (MCP) server to connect other tools.
- Manage sign-in, multifactor authentication, and user access through a bundled Keycloak service.

It does **not** include URL or YouTube transcription, privileged recording, billing or credit purchases, self-service sign-up, social login, or email workflows.

## How your data moves

- Audio you upload or record is checked for malware and stored encrypted on your server.
- For transcription, the server sends the audio to Speechmatics under your own Speechmatics account. It uses a generic filename, not the original one.
- The default Speechmatics endpoint is in the EU (`eu1.asr.api.speechmatics.com`). You can choose another Speechmatics region.
- Speechmatics returns the transcript. Voxis then asks Speechmatics to delete the job, and retries later if that fails.
- Transcripts are stored encrypted on your server.
- Summaries are written by the Gemma model on your own hardware. The transcript text goes only to that local model.
- Logs record IDs, status, and error categories. They do not record filenames, titles, or transcript text.
- Deleting a recording removes its audio, transcripts, summaries, and file details from the server. Copies in existing backups remain until those backups are deleted.

The [data-handling guide](docs/data-handling.md) covers each step in detail.

## What you need

- A Linux x86_64 server with Docker Engine and the Docker Compose plugin. The media sandbox is tested on Debian 12. Docker Desktop is not supported for production use.
- A Speechmatics account and API key. You pay Speechmatics directly and accept its terms.
- Enough hardware for the Gemma 4 12B model. Our tested setup used 8 CPU threads and about 32 GB of RAM, with about 7 GB of disk for the model. The bundled setup runs the model on CPU, so a summary of a long transcript can take many minutes.
- Disk space for audio, scratch files, Docker images, and backups. The installation guide lists the minimums.
- A domain name and HTTPS certificate, so browsers can sign in securely.

## Install

Follow the [installation guide](infra/oss/README.md). It covers setup, the first administrator, user access, key storage, backups, and restores. This release needs a fresh installation: upgrading from, or restoring a backup of, an earlier pre-release candidate is not supported.

```bash
git clone https://github.com/nyata-ai/voxis-source-available.git
cd voxis-source-available/infra/oss
```

Users need the `voxis-user` role in Keycloak to use the app. Administrators also need `voxis-admin`. Someone without the role sees a message asking them to contact their administrator.

## Security and privacy

What the installation does:

- Stored audio, transcripts, and summaries are encrypted with per-organization keys held in OpenBao.
- The OpenBao root token is revoked after setup. The unseal key shares are meant to be held by named people, away from the server, and are not included in backups.
- The Keycloak admin console is not exposed to the internet. Administrators reach it over an SSH tunnel.
- Backups are encrypted with `age`.
- Containers are split into networks. The databases, key store, and AI model have no internet access.
- Uploaded media is parsed inside a sandbox. Operators can turn this off, but we do not recommend it.
- The web app sends security headers, including a Content Security Policy.
- API keys stop working within about a minute after a user is disabled in Keycloak or loses the `voxis-user` role.

Honest limits:

- **Speechmatics is a cloud service.** It receives your audio. Check that this is acceptable for your recordings.
- **Server administrators are trusted.** Anyone with administrator access to the server can reach the running app and its keys, and therefore the data.
- **Some data is not encrypted.** Filenames, titles, descriptions, account details, and job records are stored as ordinary database data.
- **Encrypted fields are not tied to their records.** Someone who can write to the database could swap encrypted values between records within the same organization.
- **Backups are encrypted but not signed.** Keep them off the server, ideally on write-once storage.
- **Release images are not signed.** Build from source or check image digests yourself.
- **Browser recording backup is light protection.** Unsent recording pieces are encrypted in the browser, but the key is stored in the same browser. They are cleared at sign-out.

See [data handling](docs/data-handling.md), [capability limits](docs/capability-limits.md), and [model validation](docs/model-validation.md).

## License in brief

The [Voxis Source-Available License 1.0](LICENSE) is the binding text. In short:

- **Free for personal, noncommercial use** by individuals.
- **Free for the own operations of educational institutions** of any kind: public or private, nonprofit or for-profit, including government-run schools and universities.
- **Free for the own operations of nonprofit organizations and religious institutions** that are not part of government. This includes informal community and charitable groups that do not distribute profits.
- **Contractors and volunteers** may install and run it for one of these users, only for that user's own operations. They may charge for their work, but may not resell the software or host it for several customers as a commercial service.
- **Anyone may evaluate it**: inspect, build, test, and modify it to check security or decide on a purchase.
- **A commercial license is needed** for business operations, client work, resale, commercial hosting, and government use outside education.

If you share copies, keep the license and notices, mark your changes, and do not charge for the software. Third-party services, models, container images, and dependencies keep their own terms; see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Contact

- Licensing and commercial questions: [oss@nyata.ai](mailto:oss@nyata.ai)
- Security vulnerabilities: [security@nyata.ai](mailto:security@nyata.ai). Please read [SECURITY.md](SECURITY.md) first.

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request. It explains the contribution license you grant and the rule that test data must be synthetic.

## Further reading

- [Installation guide](infra/oss/README.md)
- [Data handling and trust boundaries](docs/data-handling.md)
- [Capability limits](docs/capability-limits.md)
- [Model validation](docs/model-validation.md)
- [Acceptance evidence](docs/acceptance-evidence.md)
- [Roadmap](docs/roadmap.md)
- [Third-party notices](THIRD_PARTY_NOTICES.md)
