# Upstream schema map

Voxis Source-Available creates a new database with
`backend/db/migrations/000001_initial.up.sql`. It does not apply, renumber, or
convert the migration history of Nyata's commercial Voxis source. Existing
commercial databases are not supported as inputs.

The review covered every upstream `*.up.sql` migration at the source revision
recorded in `UPSTREAM.json`. “Retained” means its final schema effect is present
in the initial migration. “Adjusted” states the deliberate difference in this
edition. Migrations for capabilities outside this edition are grouped under
[Excluded migrations](#excluded-migrations).

| Upstream migration | Treatment | Initial-schema result |
| --- | --- | --- |
| 000001 create extensions | Retained | `uuid-ossp`, `pgcrypto`, and `pg_trgm` are created. |
| 000002 create organizations | Retained | `organizations` is created with the persistent transit key reference. |
| 000003 create users | Retained | `users` is created with organization ownership and preferences. |
| 000004 create media | Retained | Encrypted uploaded and recorded media metadata is created. |
| 000005 create transcriptions | Adjusted | Media-backed transcriptions only; the provider check permits only `speechmatics`. |
| 000006 create summaries | Retained | Encrypted summaries are created. |
| 000007 create API keys | Retained | Scoped API keys and active-key indexes are created. |
| 000008 multi-language transcriptions | Retained | The `languages` array is created. |
| 000010 add media metadata | Retained | Title, description, file hash, and audio metadata fields are created. |
| 000011 cleanup orphan soft deletes | Adjusted | Fresh-schema foreign keys, status checks, and active-row indexes avoid the historic cleanup step. |
| 000012 add Gladia deleted at | Adjusted | Replaced by `speechmatics_deleted_at`. |
| 000013 add Gladia cleanup index | Adjusted | Replaced by Speechmatics stale-job and undeleted-job indexes. |
| 000014 create recording sessions | Adjusted | Standard recording sessions and chunks are created; privileged recording state is absent. |
| 000015 create transcription segments | Adjusted | Media-backed Speechmatics segment rows are created. |
| 000016 encrypt transcription segment content | Retained | Segment ciphertext envelope columns are created. |
| 000017 add recording last activity | Retained | Recording activity fields and indexes are created. |
| 000019 add media scan status | Retained | Scan gating fields and pending-scan index are created. |
| 000021 add preprocessing fields | Retained | Enhancement request and actual preprocessor fields are created. |
| 000022 add thinking tokens | Retained | Summary usage fields are created. |
| 000023 add search indexes | Adjusted | Media metadata trigram indexes are created. Transcript search decrypts bounded owned records and never indexes plaintext transcript content. |
| 000025 add summary high stakes | Retained | High-stakes summaries and their index are created. |
| 000026 add dashboard stats index | Retained | Retained media, transcription, and summary indexes support operational statistics. |
| 000027 create MCP collections | Retained | MCP collections and collection items are created. |
| 000029 create LLM prompt overrides | Retained | `llm_prompt_overrides` is created for public Gemma prompt controls. |
| 000030 add recording capture source and retention | Adjusted | Standard capture source and retention data are created; privileged capture variants are absent. |
| 000031 create global recording retention policy | Retained | `app_recording_retention_policy` is created and seeded. |
| 000032 drop org recording retention columns | Retained | Only the application-wide retention policy is present. |
| 000037 UI language means chosen | Retained | User preferences remain JSONB and carry the selected UI language. |
| 000040 add summary structured metadata | Retained | Structured result, source hash/version, and prompt provenance are created. |
| 000041 add summary degradation codes | Retained | Summary degradation metadata is created. |
| 000042 add user storage quota | Retained | Application quota policy and user allocation rows are created. Quotas are not enforced in this edition. |
| 000043 add user normalized email | Retained | `normalized_email` and its index are created. |
| 000044 org wrapped KEKs | Adjusted | This edition uses persistent transit key references and transit historical versions. It does not create the commercial wrapped-KEK table. |
| 000045 add transcription vocabulary packs | Adjusted | The compatibility column remains in the fresh schema, but custom vocabulary is disabled because Speechmatics Melia does not support it. |
| 000046 add Speechmatics provider | Retained | Speechmatics job identifiers, deletion markers, and cleanup indexes are created. |
| 000047 add transcription expected speakers | Retained | Bounded expected-speaker fields are created. |
| 000049 add summary model metadata | Retained | Model, model revision, runtime revision, quantization, and measured-usage availability metadata are created. |

## Excluded migrations

Twelve upstream migrations belong to capabilities that this edition does not
include. None of their tables, columns, or indexes are created.

| Capability | Migrations | Result in this edition |
| --- | --- | --- |
| Billing, payments, credits, and usage charges | 7 | No billing, payment, credit, reservation, sweep, or usage-charge schema. |
| URL transcription | 3 | No URL source fields, metadata, or search index. |
| Email verification | 1 | Sign-in relies on the bundled OpenID Connect issuer. |
| Privileged recording | 1 | No privileged recording mode or related schema. |

## Changes after the first release

Before the first release, this map and the fresh-install baseline may change
together. After that release, every retained schema change needs a new
append-only migration in this edition's own sequence. Review it against this map
and test both a fresh install and an upgrade from the previous release.
Commercial databases remain unsupported as inputs.
