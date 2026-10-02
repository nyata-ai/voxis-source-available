# Third-party intake rules

Every file added to Voxis Source-Available needs a known source, a version, a rights classification, and a reason for inclusion. Preserve every copyright and third-party notice.

- **Fonts.** Proprietary fonts are excluded. The web app uses system font stacks.
- **Separate intake classes.** Treat images, screenshots, audio, model weights, container images, media binaries, and code dependencies as separate classes. Each needs its own license check. Being tracked in version control does not by itself give a right to publish.
- **Models.** Google's Gemma QAT GGUF repository is published under Apache-2.0. Model terms, the llama.cpp runtime, and container image licenses still need their own notices.
- **Build tools.** A permissive license for a build tool does not automatically cover a binary it downloads or produces.

Record the result in [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md).
