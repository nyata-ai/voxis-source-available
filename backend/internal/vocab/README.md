# Built-in vocabulary packs

## Purpose

Each JSON file in `packs/` is a curated list of domain terms that the user can
attach to a transcription at submission time. The worker resolves the selected
pack ids into terms and sends them to the transcription provider as
`custom_vocabulary_config`, biasing the decoder towards spellings it would
otherwise get wrong (Indonesian legal vocabulary, distressed-debt jargon,
acronyms).

Pack ids — never the terms — are what get persisted on the transcription row.
The terms themselves are public, built-in content, so nothing
client-confidential is stored in plaintext or sent beyond the audio's existing
zero-retention path. Ad-hoc free-text terms are deliberately **not** supported:
those would be client data and would need envelope encryption before a worker
could read them back.

## Packs

| id | Scope |
|---|---|
| `sea-restructuring` | Indonesian insolvency (PKPU, homologasi, kurator), Indonesian criminal procedure (BAP, tersangka, penyidik), Indonesian institutions (OJK, LPS, KPK, PPATK), Singapore (IRDA, judicial management) and Malaysia (CDRC, PN17, restraining order) |
| `distressed-core` | Cross-regional distressed debt and leveraged finance (PIK, DIP, LME, uptier, fulcrum security) |

## Authoring rules

- **Shape.** `{ "id", "description", "terms": [ { "value", "pronunciations"?, "intensity"?, "language"?, "gloss"? } ] }`.
  The `id` must match the filename stem. `gloss` is documentation for
  maintainers only — it is stripped before the term reaches the provider.
- **`value`** is the spelling you want in the transcript. Max 64 runes.
- **`pronunciations`** are alternative spoken forms; add them for acronyms said
  as letters or as a word. Max 5 per term, 64 runes each. Use Indonesian letter
  names for Indonesian acronyms (`PKPU` -> `peh kah peh uh`, `BAP` -> `bahp`
  plus `beh ah peh`), English letter names for English ones.
- **`language`** is an ISO 639 code. Set `id` on Indonesian terms; leave it
  empty for English terms so they are not scoped to one language.
- **`intensity`** is normally omitted. The provider's default (0.5) is what we
  want, and we deliberately never send `default_intensity`. Values are clamped
  to [0,1] at load.
- **No duplicates** within a pack. Across packs, duplicate values collapse to
  the first occurrence in canonical (alphabetical) pack order.
- Keep files ASCII where the term allows it, and keep terms genuinely ambiguous
  to a speech model — a common English word gains nothing and can cause
  false substitutions.

## Caps

Enforced in `vocab.go`, not by the provider (which documents no hard limit):
6 packs per request, 500 merged terms per request, 1000 terms per pack.

A malformed pack or term is dropped rather than fatal; `LoadIssues()` reports
what was rejected and `TestPacks_LoadCleanly` fails the build if anything is.

## Sources and review cadence

Authored 2026-08-30 from the regional restructuring/insolvency terminology
research that accompanied `docs/plans/2026-08-30-meeting-profiles-vocabulary-bap-plan.md`
(Phase C). Sources: Indonesian Law 37/2004 (PKPU/bankruptcy) and KUHAP
(UU 8/1981, as replaced by UU 20/2025) terminology, Singapore IRDA 2018,
Bursa Malaysia PN17 and BNM CDRC materials, and standard distressed-debt
market usage.

**Review every 6 months**, and whenever a named institution or statute changes:

- Legal vocabulary is time-sensitive. Korea's CRPA sunsets at end-2026 — no
  Korean terms are in these two packs, so that is a gate on authoring a
  `north-asia` pack, not a change here.
- Hong Kong "provisional supervision" is deliberately **excluded**: the bill is
  unenacted, and shipping unenacted terminology teaches the model a spelling
  for something practitioners do not yet say.
- Indonesian institution names change with policy (Danantara is recent). Check
  the institution list first at each review.

The remaining researched packs (`north-asia`, `europe-restructuring`,
`middle-east`, and splitting `id-criminal-procedure` out of
`sea-restructuring`) are intentionally not shipped yet: they are only worth the
maintenance once these two prove the mechanism.
