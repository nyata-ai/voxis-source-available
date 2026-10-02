# Contributing to Voxis Source-Available

Thank you for helping. Voxis Source-Available is source-available software under the [Voxis Source-Available License](LICENSE). Maintainers review every pull request. Opening one does not guarantee that it will be merged.

For a security problem, do not open a pull request or issue. Follow [SECURITY.md](SECURITY.md) instead.

## Before you submit

- Keep each change small and focused. Explain what it changes, the tests you ran, and anything you could not test.
- Use synthetic audio, transcripts, names, identities, and provider responses only. Never submit real recordings, real transcripts, personal data, credentials, tokens, private hostnames, or production configuration.
- If you add code, assets, models, or dependencies from elsewhere, state the source, version, and license. The license must allow distribution in this project. A permissive license for a build tool does not automatically cover what that tool downloads or produces.
- Do not add proprietary fonts. The web app uses system fonts.
- Test on the supported Linux container setup when your change affects containers, media tools, sign-in, storage, backups, or recovery. A Windows-only or macOS-only check is not enough for those areas.
- Update the documentation when a workflow or data boundary changes. User-visible changes need the in-app guide updated in all four guide languages (English, Indonesian, German, and Simplified Chinese).

This edition deliberately leaves out URL transcription, privileged recording, billing, payments, credit purchasing, Gemini controls, self-service sign-up, social login, Turnstile, and contact-email workflows. Pull requests that add them will not be accepted.

## Contribution license

By submitting a contribution, you represent that you have the right to submit it. You grant PT. Karya Nyata Teknologi a perpetual, worldwide, non-exclusive, royalty-free, irrevocable copyright license to use, reproduce, modify, prepare derivative works of, sublicense, distribute, publicly display, and publicly perform that contribution in Voxis Source-Available and in commercial Voxis products, under any terms.

You also grant PT. Karya Nyata Teknologi a perpetual, worldwide, non-exclusive, royalty-free, irrevocable patent license under patent claims that you can license and that are necessarily infringed by the contribution alone or by its combination with the project. The patent license permits making, having made, using, offering to sell, selling, importing, and otherwise transferring the contribution and those combinations.

You keep ownership of your contribution. This grant does not override a third-party license that you are not authorized to relicense. Do not submit code or material with terms that conflict with this grant. We may accept, reject, modify, or remove a contribution.

### How you accept it

Every pull request must include this statement in its description, with the box checked:

> - [x] I have read the Contribution license section of CONTRIBUTING.md. I have the right to submit this contribution, and I grant the licenses described there.

The [pull request template](.github/pull_request_template.md) includes it. If you contribute on behalf of an employer or another organization, make sure you are authorized to grant these licenses for it. Maintainers will not merge a pull request without the checked statement. The grant also applies to any contribution you submit in another way, such as a patch sent by email.

## Review standards

- Keep changes clear and narrow. Do not reformat or refactor unrelated code.
- Keep the exact names, terms, and limits of configured components.
- Handle errors and promise rejections that users could see.
- Do not silence tests, linters, or warnings to make a check pass.

See also [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and [docs/data-handling.md](docs/data-handling.md).
