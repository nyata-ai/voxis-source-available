import { LANGUAGES, getLanguageLabel } from '@/lib/languages'
import type { Utterance } from '@/types/transcription'

type TranslateFn = (key: string, options?: Record<string, unknown>) => string

/** What a transcript row shows in its meta column besides the timestamp. */
export interface TurnMeta {
  /** First turn of a run by one speaker: the name and disc show. */
  showSpeaker: boolean
  /** "ID → EN" when this turn's language tag differs from the last tagged
   *  turn before it. Absent when either tag is missing: no tag, no claim. */
  switchText?: string
  /** The same switch spelled out for a screen reader and the tooltip. */
  switchLabel?: string
}

/** Melia tags Chinese by its spoken variety. The app, its CJK type and its
 *  language names all know it as `zh`. */
const CHINESE_VARIETIES = new Set(['cmn', 'yue'])

/** The primary subtag, lower-cased: "en-US" and "en" are one language here, and
 *  so are "cmn" and "zh". Empty for a missing or blank tag. */
export function languageKey(tag: string | undefined): string {
  const primary = (tag ?? '').trim().toLowerCase().split(/[-_]/)[0] ?? ''
  return CHINESE_VARIETIES.has(primary) ? 'zh' : primary
}

/** The tag for a `lang` attribute. It keeps the rest of the tag ("cmn-Hant" is
 *  "zh-Hant"), because the script picks the font. Undefined for a blank tag. */
export function htmlLang(tag: string | undefined): string | undefined {
  const trimmed = (tag ?? '').trim()
  if (!trimmed) return undefined
  const [primary = '', ...rest] = trimmed.split(/[-_]/)
  return CHINESE_VARIETIES.has(primary.toLowerCase()) ? ['zh', ...rest].join('-') : trimmed
}

/** A readable name for a language tag: the app's own translated name when the
 *  language is one it offers, the browser's name for anything else, and the
 *  bare code when neither knows it. */
export function languageName(key: string, t: TranslateFn, locale: string): string {
  if (LANGUAGES.some((language) => language.value === key)) return getLanguageLabel(key, t)
  try {
    const name = new Intl.DisplayNames([locale], { type: 'language' }).of(key)
    if (name && name.toLowerCase() !== key) return name
  } catch {
    // An unknown locale or a malformed tag: fall through to the code.
  }
  return key.toUpperCase()
}

/**
 * One pass over the transcript: where each speaker's run starts, and where the
 * provider's language tag changes. Gladia tags each turn; for Melia the backend
 * sets the turn's tag to the majority language of its words. A turn without a
 * tag neither shows a switch nor breaks one: the comparison carries on from
 * the last tagged turn.
 */
export function buildTurnMeta(
  utterances: Utterance[],
  t: TranslateFn,
  locale: string,
): TurnMeta[] {
  const meta: TurnMeta[] = new Array(utterances.length)
  let previousLanguage = ''
  for (let i = 0; i < utterances.length; i++) {
    const utterance = utterances[i]!
    const showSpeaker = i === 0 || utterances[i - 1]!.speaker !== utterance.speaker
    const language = languageKey(utterance.language)
    const entry: TurnMeta = { showSpeaker }
    if (language && previousLanguage && language !== previousLanguage) {
      entry.switchText = `${previousLanguage.toUpperCase()} → ${language.toUpperCase()}`
      entry.switchLabel = t('transcription:viewer.languageSwitch', {
        from: languageName(previousLanguage, t, locale),
        to: languageName(language, t, locale),
      })
    }
    if (language) previousLanguage = language
    meta[i] = entry
  }
  return meta
}

/**
 * The turn a citation at `time` lands on: the one playing at that moment, or
 * failing that (a time in a pause) the first turn after it, or the last turn.
 * -1 for an empty transcript.
 */
export function findTurnAt(utterances: Utterance[], time: number): number {
  if (utterances.length === 0) return -1
  let after = -1
  for (let i = 0; i < utterances.length; i++) {
    const utterance = utterances[i]!
    if (time >= utterance.start && time < utterance.end) return i
    if (after === -1 && utterance.start >= time) after = i
  }
  return after === -1 ? utterances.length - 1 : after
}
