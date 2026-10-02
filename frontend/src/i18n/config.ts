export interface LocaleMeta {
  code: string
  label: string // English name
  nativeLabel: string // endonym, shown in the picker
  shortCode: string // 2-letter code shown in the compact header switcher
}

export const SUPPORTED_LOCALES: LocaleMeta[] = [
  { code: 'en', label: 'English', nativeLabel: 'English', shortCode: 'EN' },
  { code: 'id', label: 'Indonesian', nativeLabel: 'Bahasa Indonesia', shortCode: 'ID' },
  { code: 'zh-CN', label: 'Chinese (Simplified)', nativeLabel: '简体中文', shortCode: 'ZH' },
  { code: 'ja', label: 'Japanese', nativeLabel: '日本語', shortCode: 'JA' },
  { code: 'ko', label: 'Korean', nativeLabel: '한국어', shortCode: 'KO' },
  { code: 'es', label: 'Spanish', nativeLabel: 'Español', shortCode: 'ES' },
  { code: 'de', label: 'German', nativeLabel: 'Deutsch', shortCode: 'DE' },
  { code: 'fr', label: 'French', nativeLabel: 'Français', shortCode: 'FR' },
  { code: 'ru', label: 'Russian', nativeLabel: 'Русский', shortCode: 'RU' },
]

export const SUPPORTED_LANGUAGE_CODES = SUPPORTED_LOCALES.map((l) => l.code)

/**
 * Locales offered in the language pickers (header switcher, Settings, landing).
 * Catalogs for the other codes still ship — a reader whose saved ui_language is
 * outside this list keeps their language and sees it appended to the picker —
 * but new selection is limited to these four.
 */
export const PICKER_LOCALE_CODES = ['en', 'id', 'zh-CN', 'de'] as const
export const PICKER_LOCALE_CODE_SET = new Set<string>(PICKER_LOCALE_CODES)

/**
 * The locales a language picker should offer, in SUPPORTED_LOCALES order:
 * the four picker locales, plus the active locale when it is outside that
 * list — the control must keep reporting the language the app is actually
 * shown in, and give its reader a way to switch away. The single source for
 * all three pickers (header switcher, Settings, landing).
 */
export function pickerLocales(active: string): LocaleMeta[] {
  return SUPPORTED_LOCALES.filter((l) => PICKER_LOCALE_CODE_SET.has(l.code) || l.code === active)
}
export const TRANSLATED_LANGUAGE_CODES = [
  'en',
  'id',
  'zh-CN',
  'ja',
  'ko',
  'es',
  'de',
  'fr',
  'ru',
] as const
export const TRANSLATED_LANGUAGE_CODE_SET = new Set<string>(TRANSLATED_LANGUAGE_CODES)
/** A locale this build actually ships a catalog for. */
export type TranslatedLanguageCode = (typeof TRANSLATED_LANGUAGE_CODES)[number]
export const DEFAULT_LOCALE = 'en'

export const NAMESPACES = [
  'common',
  'settings',
  'media',
  'transcription',
  'summary',
  'recording',
  'auth',
  'errors',
  'oss',
] as const
export type Namespace = (typeof NAMESPACES)[number]
export const DEFAULT_NS: Namespace = 'common'

/**
 * Which language wins — the whole rule, which otherwise has to be reassembled
 * from four files. Highest precedence first:
 *
 *   1. LANGUAGE_STORAGE_KEY in localStorage — a deliberate choice. Written ONLY
 *      by changeAppLanguage() (i18n/language.ts), which only the three language
 *      controls call; detection never caches into it (i18n/index.ts sets
 *      `caches: []`), so its presence always means "the reader picked this".
 *   2. The browser language list, normalised to a shipped code — except an
 *      English-first list inside a mapped timezone, which yields to 3
 *      (i18n/detectors.ts).
 *   3. The browser's IANA timezone, for the three target markets
 *      (i18n/geo-language.ts).
 *   4. Whatever else navigator reports, via i18next's own detector.
 *   5. DEFAULT_LOCALE.
 *
 * Then, for a signed-in reader only, the server's ui_language is applied on top
 * of the result by useSyncLanguage() — per session and WITHOUT persisting, so it
 * never turns into rule 1.
 */

/**
 * localStorage key holding a DELIBERATE language choice.
 *
 * Bumped to -v2 when detection stopped auto-caching. The old key was written by
 * i18next with whatever it had *detected*, so every past visitor has one; under
 * the new rule that value would be misread as a choice and would lock those
 * visitors out of geo detection permanently.
 */
export const LANGUAGE_STORAGE_KEY = 'voxis-lang-v2'

/** The pre-v2 key, removed on boot so it does not linger. */
export const LEGACY_LANGUAGE_STORAGE_KEY = 'voxis-lang'
