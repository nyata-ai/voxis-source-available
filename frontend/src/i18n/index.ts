import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import resourcesToBackend from 'i18next-resources-to-backend'
import {
  TRANSLATED_LANGUAGE_CODES,
  TRANSLATED_LANGUAGE_CODE_SET,
  DEFAULT_LOCALE,
  DEFAULT_NS,
  NAMESPACES,
  LANGUAGE_STORAGE_KEY,
  LEGACY_LANGUAGE_STORAGE_KEY,
} from './config'
import { preferredNavigatorDetector, timezoneDetector } from './detectors'
import { landingLocale } from './landing-locale'

import enCommon from './locales/en/common.json'
import enSettings from './locales/en/settings.json'
import enMedia from './locales/en/media.json'
import enTranscription from './locales/en/transcription.json'
import enSummary from './locales/en/summary.json'
import enRecording from './locales/en/recording.json'
import enAuth from './locales/en/auth.json'
import enErrors from './locales/en/errors.json'
import enOss from './locales/en/oss.json'

export const enResources = {
  common: enCommon,
  settings: enSettings,
  media: enMedia,
  transcription: enTranscription,
  summary: enSummary,
  recording: enRecording,
  auth: enAuth,
  errors: enErrors,
  oss: enOss,
}

// Guard (Power-of-10 rule 5): the statically-bundled English resources must cover every
// namespace. If a namespace is ever added to NAMESPACES without a matching English import,
// that namespace silently falls through to the lazy backend and loses its synchronous English
// fallback. Fail fast at startup instead of shipping a missing-fallback footgun.
const bundledNamespaces = Object.keys(enResources)
if (
  bundledNamespaces.length !== NAMESPACES.length ||
  !NAMESPACES.every((ns) => bundledNamespaces.includes(ns))
) {
  throw new Error(
    `i18n: bundled English namespaces [${bundledNamespaces.join(', ')}] do not match NAMESPACES [${NAMESPACES.join(', ')}]`
  )
}

const lazyBackend = resourcesToBackend(
  (lng: string, ns: string) => import(`./locales/${lng}/${ns}.json`)
)

// Custom detectors must be registered on an INSTANCE before init(): i18next's
// createClassOnDemand calls `new` on a class, so `.use(LanguageDetector)` would
// leave no handle to register on, and detection runs synchronously inside init().
const languageDetector = new LanguageDetector()
languageDetector.addDetector({
  name: 'landingPath',
  lookup: () => landingLocale(window.location.pathname),
})
languageDetector.addDetector(preferredNavigatorDetector)
languageDetector.addDetector(timezoneDetector)

/**
 * Bring the stored language keys up to what this build understands.
 *
 * Runs once below, before init() reads localStorage. Exported only so a test can
 * drive it — nothing else should call it.
 *
 * Storage access must never throw here: this executes at module scope, and
 * main.tsx imports ./i18n before createRoot().render(), so an exception escapes
 * with no error boundary mounted and the visitor gets a blank page.
 */
export function pruneStoredLanguage() {
  try {
    // One-shot migration off the pre-v2 key, but it runs on every boot forever.
    // Safe to delete once no returning device can still hold that key — a year
    // after this ships (2027-08) is the marker; the only cost of being late is
    // one wasted removeItem() per boot.
    localStorage.removeItem(LEGACY_LANGUAGE_STORAGE_KEY)

    // changeAppLanguage() coerces on the way in, but i18next reads this key raw,
    // and localStorage outlives the build that wrote it. Retiring or renaming a
    // code in TRANSLATED_LANGUAGE_CODES leaves every reader who chose it holding
    // a value nothing validates on the way out. i18next drops it from its exact
    // pass, but not from the fuzzy pass that runs when nothing matched exactly —
    // where a retired 'zh-CN' would match a newly shipped 'zh-TW', handing a
    // Simplified reader Traditional. Clearing it keeps the invariant "this key
    // holds a shipped code" true at the one place that can still enforce it.
    const stored = localStorage.getItem(LANGUAGE_STORAGE_KEY)
    if (stored !== null && !TRANSLATED_LANGUAGE_CODE_SET.has(stored)) {
      localStorage.removeItem(LANGUAGE_STORAGE_KEY)
    }
  } catch {
    // Private mode or blocked storage — nothing to clean up.
  }
}
pruneStoredLanguage()

i18n
  .use(lazyBackend)
  .use(languageDetector)
  .use(initReactI18next)
  .init({
    resources: { en: enResources },
    partialBundledLanguages: true,
    fallbackLng: DEFAULT_LOCALE,
    supportedLngs: [...TRANSLATED_LANGUAGE_CODES],
    ns: NAMESPACES,
    defaultNS: DEFAULT_NS,
    interpolation: { escapeValue: false },
    react: { useSuspense: false },
    detection: {
      // NOT a short-circuit chain: i18next concatenates every detector's output
      // into one array and picks the first EXACT supportedLngs member. Each
      // detector therefore returns a shipped code or nothing. See detectors.ts.
      //
      // Trailing 'navigator' is i18next's own detector, kept for the two inputs
      // preferredNavigator cannot reach: navigator.languages entries past its
      // 10-tag cap, and navigator.language when navigator.languages is not an
      // array at all — there preferredNavigator reports nothing by design, and
      // the built-in still recovers a real answer instead of a blind fallback.
      order: ['landingPath', 'localStorage', 'preferredNavigator', 'timezone', 'navigator'],
      lookupLocalStorage: LANGUAGE_STORAGE_KEY,
      // Disabled deliberately. i18next caches the DETECTED language, which would
      // make the first guess indistinguishable from a deliberate choice forever.
      // changeAppLanguage() is the sole writer, called only by the three language
      // controls; applyServerLanguage() switches without writing — see language.ts.
      caches: [],
    },
  })

/**
 * Keep <html lang> naming the language actually on screen.
 *
 * Deliberately NOT the code `languageChanged` carries: that is the REQUESTED
 * language, and when a lazy catalog chunk fails to load i18next still moves
 * `language` to it while `resolvedLanguage` stays on the fallback that is really
 * being rendered. lang="de" over English text misleads screen readers and any
 * language-sniffing crawler. i18next recomputes resolvedLanguage before it emits,
 * so reading it here is never one switch stale (pinned by a test in language.test.ts).
 */
function applyHtmlLang() {
  if (typeof document === 'undefined') return
  document.documentElement.setAttribute('lang', i18n.resolvedLanguage || DEFAULT_LOCALE)
}
i18n.on('languageChanged', applyHtmlLang)
applyHtmlLang()

/**
 * Print every detection input and what came of them, on one line.
 *
 * All four inputs are client-side and none reach the server, so a "why is my UI
 * in Indonesian?" report otherwise costs a devtools walkthrough. Reads the same
 * three sources the detectors do, plus the language i18next settled on.
 *
 * Exported for the test; the only caller is the DEV-gated line below.
 */
export function logLanguageDetection() {
  try {
    const stored = localStorage.getItem(LANGUAGE_STORAGE_KEY)
    // Not navigator.languages directly: anti-fingerprinting extensions substitute
    // shapes the type says are impossible, and a diagnostic must never be the
    // thing that breaks the boot it is diagnosing (see detectors.ts).
    const languages = Array.isArray(navigator?.languages)
      ? navigator.languages.join(',')
      : String(navigator?.languages)
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
    console.info(
      `[i18n] stored=${stored ?? '-'} navigator=${languages} timezone=${zone ?? '-'} resolved=${i18n.resolvedLanguage}`
    )
  } catch {
    // Storage, navigator and Intl can all throw in locked-down browsers.
  }
}

// DEV only, so it never ships. The MODE check keeps it out of the test run,
// where tests/setup.ts imports this module for all 150+ files.
if (import.meta.env.DEV && import.meta.env.MODE !== 'test') {
  logLanguageDetection()
}

export default i18n
