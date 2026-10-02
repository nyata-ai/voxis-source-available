# Model validation

This page explains which summary model Voxis Source-Available uses, why it was chosen, and what has and has not been checked. All checks used synthetic material only; no real transcript was used. Detailed measurements are in [acceptance evidence](acceptance-evidence.md).

## Default model

| Item | Value |
| --- | --- |
| Model | `google/gemma-4-12B-it`, official QAT Q4_0 GGUF variant `google/gemma-4-12B-it-qat-q4_0-gguf` |
| Model revision | `29d097773436b69ff9feafd636ab4cf873786537` |
| Model file SHA-256 | `93567e57a8fe10b23569b9d9ec38cd005deedf71e29477c421a4b83f418a538b` |
| Runtime | llama.cpp, commit `5266f24da75dc449bd56cbed7addb9c8e4a6a73e` |
| Bundled context | 16,384 tokens, on CPU |

These values match the defaults in `infra/oss/.env.example`. The admin page shows the model, runtime, revision, and quantization that the server reports.

## How the model was chosen

Two Gemma 4 models were compared on the same GPU with the same settings and the same prompts:

- **Gemma 4 12B QAT Q4_0** passed all eight short synthetic test transcripts in English, Indonesian, German, and Chinese. A separate review found that its summaries kept the facts, dates, numbers, and uncertainty in the source. It was selected as the default.
- **Gemma 4 26B-A4B QAT Q4_0** passed six of the eight. Two summaries were rejected for invalid citations. It is kept as a comparison point only and is not a supported runtime.

This comparison does not claim semantic scoring, parity with Gemini, or proof for every summary type.

## What else was checked

- **Prompt behaviour.** Later prompt revisions tightened how speakers, questions, dates, owners, decisions, and open issues are handled. On the synthetic tests, question-and-answer output kept the real questions, left speaker fields empty when the source had no speaker names, and did not guess who was speaking. Earlier failures were kept as evidence; validators were not loosened to make results pass.
- **Long input (GPU, 32K context).** A five-chunk synthetic source kept facts and decisions from the last chunk in both standard and high-stakes summaries. This used test storage, not the full application.
- **Full application (CPU, bundled 16K setup).** A four-chunk synthetic source went through the whole application, including encrypted storage, and the saved summary kept facts and a citation from late in the source. A short, one-chunk high-stakes summary also completed through the full application.
- **Structure limits.** Automated tests cover how key points, action items, and question-and-answer entries are shared out and merged across up to five chunks. Input beyond five chunks fails before the model is called.

## Not yet established

- Summary quality on real meetings.
- Speed on typical hardware, or a throughput target.
- Citations grounded to exact timestamps or speakers.
- Anchored (high-stakes) summaries across several chunks, and long anchored summaries on CPU.
- A 32K context on the bundled CPU setup.

Operators who need any of these should test them on their own hardware and with their own content before relying on them.
