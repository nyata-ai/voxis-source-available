import type { CustomDetector } from 'i18next-browser-languagedetector'
import { TRANSLATED_LANGUAGE_CODES } from './config'
import { localeFromTimeZone } from './geo-language'

const SUPPORTED: readonly string[] = TRANSLATED_LANGUAGE_CODES

/**
 * The language that carries no signal about what the reader wants.
 *
 * Deliberately a literal and NOT DEFAULT_LOCALE: a deployment may set
 * DEFAULT_LOCALE to another language (say 'id'), and skipping that language
 * there would break exactly the readers the deployment serves. The rule is
 * "English is the global OS default", not "the fallback language".
 */
const NEUTRAL_LANGUAGE = 'en'

/** navigator.languages is short in practice; cap the scan (Power of 10 rule 2). */
const MAX_NAVIGATOR_LANGUAGES = 10

/**
 * Reduce a BCP-47 tag to a code that is an EXACT member of supportedLngs.
 *
 * This matters far more than it looks. i18next's detector concatenates every
 * detector's output into one array, then getBestMatchFromCodes takes the first
 * code that exactly matches supportedLngs — its base-language fallback runs only
 * when NOTHING in the array matched exactly. So returning a raw 'de-DE' here
 * would lose to a later 'id' from the timezone detector, and a German browser in
 * Jakarta would render Indonesian.
 */
export function toSupportedLocale(tag: string): string | undefined {
  if (!tag) return undefined
  const lower = tag.toLowerCase()

  const exact = SUPPORTED.find((code) => code.toLowerCase() === lower)
  if (exact) return exact

  const base = lower.split('-')[0]
  const baseMatch = SUPPORTED.find((code) => code.toLowerCase() === base)
  if (baseMatch) return baseMatch

  // 'zh' → 'zh-CN': the shipped code carries a region the browser did not.
  return SUPPORTED.find((code) => code.toLowerCase().startsWith(`${base}-`))
}

/**
 * The tag list to score, or undefined when the browser is not telling the truth.
 *
 * navigator.languages is typed `readonly string[]`, but anti-fingerprinting
 * extensions and some embedded webviews substitute shapes the type says are
 * impossible — a bare string, or an array holding numbers and nulls. Anything
 * that is not a usable array is reported as no signal, which costs nothing:
 * i18next's own `navigator` detector still runs last in `order` and reads the
 * same properties, so a truthful navigator.language is outranked, never lost.
 */
function navigatorTags(): readonly unknown[] | undefined {
  const raw: unknown = navigator.languages
  // An empty list is a real browser state (and `undefined` is a browser too old
  // to have the property): navigator.language is the older, always-present API,
  // and reading it here keeps this detector's precedence over the timezone.
  if (Array.isArray(raw)) return raw.length > 0 ? raw : [navigator.language]
  return raw === undefined ? [navigator.language] : undefined
}

/**
 * The reader's configured language, when the list says anything at all.
 *
 * Runs before the timezone detector so a Japanese visitor in Berlin keeps
 * Japanese instead of being handed German. English is the one language that can
 * yield to the timezone — see the rule inside firstPreferredCode().
 *
 * NOT independent of the timezone detector, despite what the `order` array in
 * index.ts suggests: this detector calls localeFromTimeZone() itself to decide
 * whether an English-first list should yield. The two are read in sequence by
 * i18next but coupled in that one branch, so changing the zone table changes
 * this detector's output too.
 */
export const preferredNavigatorDetector: CustomDetector = {
  name: 'preferredNavigator',
  lookup() {
    try {
      return firstPreferredCode()
    } catch {
      // Nothing in this detector may throw. Detection runs synchronously inside
      // i18next's init(), and main.tsx imports ./i18n at module scope BEFORE
      // createRoot().render() — so an exception escapes with no React error
      // boundary mounted and the visitor gets a blank page instead of the app.
      // (Privacy extensions can even install a throwing getter on
      // navigator.languages, so the property read itself is inside the try.)
      // A missing signal is always a valid answer: the next detector decides.
      return undefined
    }
  },
}

function firstPreferredCode(): string | undefined {
  if (typeof navigator === 'undefined') return undefined
  const tags = navigatorTags()
  if (!tags) return undefined
  const codes = tags
    .slice(0, MAX_NAVIGATOR_LANGUAGES)
    .map((tag) => (typeof tag === 'string' ? toSupportedLocale(tag) : undefined))
    .filter((code): code is string => Boolean(code))

  // English first in a mixed list is normally a real choice — a US reader who
  // added Spanish for web content still wants the app in English. But inside a
  // target market the timezone is the better signal: an Indonesian professional
  // on English-configured Windows is the case this feature exists for, whether
  // or not they also listed Indonesian. So English yields to geo there, and only
  // there. (`codes[0]` is undefined for an empty list, so that also yields.)
  //
  // Rejected: `codes.includes(localeFromTimeZone())`, which would send a German
  // browser in Jakarta to Indonesian — the browser language must still outrank
  // the timezone for every non-English reader.
  if (codes[0] === NEUTRAL_LANGUAGE && localeFromTimeZone()) return undefined
  return codes[0]
}

/** Where the reader is, inferred from the browser's timezone. */
export const timezoneDetector: CustomDetector = {
  name: 'timezone',
  lookup() {
    return localeFromTimeZone()
  },
}
