# Acceptance evidence

This page records the checks run on pre-release candidates of Voxis Source-Available. It is here so evaluators and auditors can see what was actually tested. It is evidence about those runs, not a guarantee about any other installation. All inputs were synthetic. Some behaviour described elsewhere in the documentation changed after these runs; this page records only what was run at the time.

## Installation and sign-in

A candidate was installed from a clean checkout on a Linux host using the Docker Compose setup. The following passed:

- HTTPS with a certificate the browser checked.
- First-user setup with password and time-based one-time password (TOTP), and a later returning-user sign-in with multifactor authentication.
- Clean and quarantined (malware test file) uploads, with scan gating before any processing.
- Browser recording, and recovery of an interrupted recording.
- Owner-only access to media and summaries.
- Library search, and JSON, PDF, and DOCX exports.
- The API key lifecycle, and a call to a retained MCP tool.
- An oversized summary source, which failed with the supported-input-size error before the model was called.

## Restore

A backup was restored onto a fresh target. The original owner signed in, and encrypted media, transcripts, and a standard summary were read back correctly. This is one exercise. It does not establish a general recovery guarantee; operators must test their own backup and restore procedures.

## Speechmatics lifecycle

One controlled run used a 2.6-second synthetic WAV file against Speechmatics' EU1 region:

- The upload passed the malware scan, and one transcription request was accepted.
- The transcription job completed on its first attempt.
- The app saved the encrypted transcript and marked the provider job as deleted. A direct request to Speechmatics for that job then returned "not found", confirming deletion.
- On readback, the transcript matched four of five expected key words; it missed one short personal name.

This confirms one narrow lifecycle. It does not measure recognition accuracy, a minimum supported duration, throughput, or behaviour across a range of audio.

## Summary model comparison (GPU)

Both candidate models ran one request at a time on a single NVIDIA L4 GPU (about 23 GB of video memory) with a 16,384-token context, eight CPU threads, full GPU offload, q8_0 key and value caches, and llama.cpp commit `5266f24da75dc449bd56cbed7addb9c8e4a6a73e`. This baseline used the earlier public prompt contract `oss-gemma-v1`.

| Model | Result |
| --- | --- |
| Gemma 4 12B QAT Q4_0 (revision `29d097773436b69ff9feafd636ab4cf873786537`) | Eight short General Summary fixtures passed in English, Indonesian, German, and Chinese. Standard calls took 7.2 to 8.8 seconds; anchored calls took 13.1 to 14.7 seconds. An independent review found the output kept the fixture facts, dates, numbers, and uncertainty. |
| Gemma 4 26B-A4B QAT Q4_0 (revision `d1c082be9cf3c8a514acf63b8761f4b41935842e`) | Six fixtures passed. Standard calls took 4.3 to 4.5 seconds; anchored calls took 7.3 to 8.6 seconds. The English and German standard calls were rejected for invalid citations after about 3.6 seconds each. |

Literal phrase checks can miss a faithful paraphrase, so the review looked at meaning as well as wording.

## Prompt and capacity checks (GPU)

Later checks used prompt contract `oss-gemma-v11`. Their timings include network transport between the test client and the GPU server, so they are not pure model latency.

- Standard and anchored question-and-answer outputs kept the real questions, left speaker fields empty when the source had no speaker names, and did not treat the person addressed as the speaker. A German hold-out test returned the expected question and answer with the stated speaker names.
- With a 32,768-token context, a five-chunk synthetic source was processed through the worker and service code with in-memory test storage. The standard summary kept a fact from chunk five (37 seconds, 15,680 prompt tokens, 638 completion tokens). The anchored high-stakes summary kept a decision from a late chunk (83 seconds, 30,645 prompt tokens, 1,532 completion tokens). The source used repetitive filler, so this is a capacity and merge check, not a meeting-quality test. It did not exercise the database, encryption, or the CPU setup.

## Full application on CPU (bundled setup)

An eight-thread CPU host with 31.35 GiB of RAM ran the bundled 16K setup.

- **Standard summary.** A 13,080-byte synthetic source of exactly four chunks went through the application, encrypted storage, and readback. The job finished on its first attempt after 1,050 seconds, using a 300-second per-request limit set for the test. The saved summary kept the source's decision, two named owners with their responsibilities and dates, an unresolved budget item, and a citation from late in the source. The source repeated filler to test capacity, so the timing is not a throughput benchmark.
- **High-stakes summary.** A short, one-chunk, text-only synthetic source was submitted through the authenticated API. The job made two model calls, of 153 and 202 seconds, on its first attempt. It saved encrypted results, and a fresh readback by the owner returned nine citations. Because the source had no audio timestamps, all citations pointed to the text-position fallback. This does not show timestamp or speaker grounding.

The bundled defaults are a 360-second per-request limit and a 60-minute summary-job limit. A long anchored summary on CPU has not been measured.

## Host measurements

During the CPU runs, about 22 GiB of memory was free before the stack started. In a steady sample, Gemma used about 7.2 GiB, and all other services together used about 2.2 GiB. The model volume was 6.50 GiB. Treat these as a starting point for sizing, not as a requirement.
