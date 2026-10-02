import i18n from './index'
import { TRANSLATED_LANGUAGE_CODE_SET, DEFAULT_LOCALE, LANGUAGE_STORAGE_KEY } from './config'

function coerce(code: string): string {
  return TRANSLATED_LANGUAGE_CODE_SET.has(code) ? code : DEFAULT_LOCALE
}

function switchTo(target: string): Promise<unknown> {
  // A failed lazy chunk (network blip / deploy-time chunk mismatch) does NOT
  // reject: measured, i18next resolves anyway, moves `language` to the requested
  // code and renders the fallback catalog, leaving `resolvedLanguage` behind — which
  // is why <html lang> reads resolvedLanguage (see index.ts). The .catch stays as
  // defence for the paths that do reject (a throwing backend, a failed re-init):
  // one place to swallow it, instead of an unhandled rejection from every
  // fire-and-forget call site.
  return i18n.changeLanguage(target).catch((err: unknown) => {
    console.error(`i18n: failed to switch language to ${target}`, err)
  })
}

/**
 * Apply a language the user deliberately chose: switch AND persist.
 *
 * Sole writer of LANGUAGE_STORAGE_KEY. i18next's own cache is switched off
 * (detection.caches: []) because it cannot tell a detected language from a chosen
 * one. Every caller of this function is asserting "the user clicked something" —
 * there are three, all language controls. Do not call it to apply a value that
 * merely arrived from somewhere; that is what applyServerLanguage is for.
 */
export function changeAppLanguage(code: string): Promise<unknown> {
  const target = coerce(code)
  try {
    localStorage.setItem(LANGUAGE_STORAGE_KEY, target)
  } catch {
    // Private mode or blocked storage: the switch still applies for this session.
  }
  return switchTo(target)
}

/**
 * Apply the server's stored preference for this session WITHOUT persisting it.
 *
 * Deliberately not a writer. An older backend build fills ui_language with "en"
 * for every user, so persisting whatever arrives would burn that default into the
 * device — and localStorage outranks detection, so it would survive the backend
 * being upgraded. Keeping this read-only makes the deploy order irrelevant and a
 * rollback harmless: the value applies for the session and leaves no residue.
 */
export function applyServerLanguage(code: string): Promise<unknown> {
  return switchTo(coerce(code))
}

/**
 * The language actually on screen — for DISPLAY only (which row is ticked, which
 * label the switcher shows).
 *
 * resolvedLanguage first, because that is the catalog being rendered: i18next
 * moves `language` to the requested code immediately but only advances
 * `resolvedLanguage` once a resource for it has loaded (and never, if the lazy
 * chunk fails). Showing the requested code would tick a row the reader is not
 * looking at.
 */
export function currentLanguage(): string {
  return i18n.resolvedLanguage || i18n.language || DEFAULT_LOCALE
}

/**
 * The language i18next is heading for — for CONTROL FLOW ("do I still need to
 * switch?").
 *
 * The two disagree for the whole of that load window, which is precisely when
 * the server preference arrives. A guard written against currentLanguage() would
 * read 'en' during an in-flight switch to 'id', conclude a stored 'en' was
 * already applied, and skip the switch it was there to make.
 */
export function requestedLanguage(): string {
  return i18n.language || i18n.resolvedLanguage || DEFAULT_LOCALE
}
