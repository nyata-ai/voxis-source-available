# Notes for AI coding agents

These notes are for contributors who use an AI coding agent on this repository. The same rules apply to human contributors. [CONTRIBUTING.md](CONTRIBUTING.md) is the full guide.

- **The contributor is responsible.** A person must review everything an agent produces and must accept the contribution license in the pull request. Only submit code you have the right to contribute.
- **Synthetic data only.** Never add real recordings, transcripts, personal data, credentials, tokens, or private hostnames, including in tests, fixtures, logs, or screenshots.
- **No paid calls in tests.** Tests must use local stubs for Speechmatics and the Gemma model. Do not make live provider calls without the contributor's explicit decision.
- **Record what you import.** For any code, asset, model, or dependency from elsewhere, state its source, version, and license.
- **Stay within scope.** Do not add URL transcription, privileged recording, billing, payments, credit purchasing, Gemini controls, self-service sign-up, social login, Turnstile, or contact-email workflows.
- **Keep the security path.** Sign-in uses the bundled Keycloak with PKCE (S256). Passwords and multifactor settings stay in the Keycloak Account Console. Do not weaken the media sandbox, encryption, or access checks to make something work.
- **Test the real platform.** Container, media-tool, sign-in, storage, and recovery changes need a check on the supported Linux container setup.
- **Update docs with behaviour.** When a user-visible workflow or data boundary changes, update the installation guide, [docs/data-handling.md](docs/data-handling.md), and all four in-app guide languages.
- **Keep checks honest.** Do not skip, silence, or weaken tests, linters, or warnings to get a green result.
