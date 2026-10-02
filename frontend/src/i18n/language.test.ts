import { afterEach, describe, expect, it, vi } from 'vitest'
import i18n, { logLanguageDetection, pruneStoredLanguage } from './index'
import {
  applyServerLanguage,
  changeAppLanguage,
  currentLanguage,
  requestedLanguage,
} from './language'
import { LANGUAGE_STORAGE_KEY, LEGACY_LANGUAGE_STORAGE_KEY, DEFAULT_LOCALE } from './config'

describe('changeAppLanguage', () => {
  afterEach(async () => {
    await changeAppLanguage('en')
  })

  it('loads shipped non-English UI catalogs', async () => {
    await changeAppLanguage('de')

    expect(currentLanguage()).toBe('de')
    expect(document.documentElement).toHaveAttribute('lang', 'de')
    expect(i18n.t('settings:apiKeys.title')).toBe('API-Schlüssel')
  })

  it('falls back to English for unsupported locale codes', async () => {
    await changeAppLanguage('it')

    expect(currentLanguage()).toBe('en')
    expect(document.documentElement).toHaveAttribute('lang', 'en')
  })

  it('records the choice in localStorage', async () => {
    localStorage.clear()
    await changeAppLanguage('de')
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe('de')
  })

  it('records the coerced value when the code is unsupported', async () => {
    localStorage.clear()
    await changeAppLanguage('xx')
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe(DEFAULT_LOCALE)
  })

  it('applyServerLanguage switches without persisting', async () => {
    localStorage.clear()
    await applyServerLanguage('de')
    expect(currentLanguage()).toBe('de')
    // The whole point: an older backend sends "en" for everyone, and persisting
    // it would outrank detection forever.
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBeNull()
  })

  it('applyServerLanguage coerces an unsupported code without persisting', async () => {
    localStorage.clear()
    await applyServerLanguage('xx')
    expect(currentLanguage()).toBe(DEFAULT_LOCALE)
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBeNull()
  })
})

/**
 * changeAppLanguage() coerces on the way in, but localStorage outlives the build
 * that wrote it: retiring or renaming a code in TRANSLATED_LANGUAGE_CODES leaves
 * every reader who chose it holding a value nothing validates on the way out.
 */
describe('pruneStoredLanguage', () => {
  afterEach(() => {
    localStorage.clear()
  })

  it('drops a stored code this build no longer ships', () => {
    localStorage.setItem(LANGUAGE_STORAGE_KEY, 'it')
    pruneStoredLanguage()
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBeNull()
  })

  it('keeps a stored code that is still shipped', () => {
    localStorage.setItem(LANGUAGE_STORAGE_KEY, 'de')
    pruneStoredLanguage()
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe('de')
  })

  it('removes the pre-v2 key', () => {
    localStorage.setItem(LEGACY_LANGUAGE_STORAGE_KEY, 'de')
    pruneStoredLanguage()
    expect(localStorage.getItem(LEGACY_LANGUAGE_STORAGE_KEY)).toBeNull()
  })

  // Runs at module scope before React mounts, so a throw here is a blank page.
  it('does not throw when storage is blocked', () => {
    const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked by policy')
    })
    try {
      expect(pruneStoredLanguage).not.toThrow()
    } finally {
      spy.mockRestore()
    }
  })
})

/**
 * All four detection inputs are client-side and none of them reach the server,
 * so without this a "why is my UI in Indonesian?" report costs a devtools
 * walkthrough. Development only — it must never ship.
 */
describe('logLanguageDetection', () => {
  afterEach(() => {
    localStorage.clear()
  })

  it('reports the four inputs and the outcome on one line', () => {
    localStorage.setItem(LANGUAGE_STORAGE_KEY, 'de')
    const spy = vi.spyOn(console, 'info').mockImplementation(() => {})
    try {
      logLanguageDetection()
      expect(spy).toHaveBeenCalledTimes(1)
      const line = String(spy.mock.calls[0][0])
      expect(line).toContain('stored=de')
      expect(line).toContain('navigator=')
      // The suite pins TZ=UTC (vitest.config.ts), so this is deterministic.
      expect(line).toContain('timezone=UTC')
      expect(line).toContain(`resolved=${i18n.resolvedLanguage}`)
    } finally {
      spy.mockRestore()
    }
  })

  it('does not throw when every input is unreadable', () => {
    const storage = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked by policy')
    })
    const spy = vi.spyOn(console, 'info').mockImplementation(() => {})
    try {
      expect(logLanguageDetection).not.toThrow()
    } finally {
      spy.mockRestore()
      storage.mockRestore()
    }
  })
})

describe('currentLanguage vs requestedLanguage', () => {
  afterEach(async () => {
    await changeAppLanguage('en')
  })

  // The two disagree for as long as the target catalog is still loading, and
  // that window is exactly when the server preference arrives. currentLanguage
  // must keep naming what is on screen; requestedLanguage is what a guard
  // deciding "do I still need to switch?" has to compare against.
  it('disagree while the requested catalog has not loaded yet', async () => {
    await changeAppLanguage('en')
    const originalResolved = i18n.resolvedLanguage
    const originalLanguage = i18n.language
    i18n.language = 'id'
    i18n.resolvedLanguage = 'en'
    try {
      expect(currentLanguage()).toBe('en')
      expect(requestedLanguage()).toBe('id')
    } finally {
      i18n.language = originalLanguage
      i18n.resolvedLanguage = originalResolved
    }
  })

  it('agree once the switch has settled', async () => {
    await changeAppLanguage('de')
    expect(currentLanguage()).toBe('de')
    expect(requestedLanguage()).toBe('de')
  })
})

describe('<html lang>', () => {
  afterEach(async () => {
    await changeAppLanguage('en')
  })

  // The premise of reading resolvedLanguage inside the languageChanged handler:
  // i18next recomputes it (setLngProps) before it emits. If that order ever
  // changes, the attribute would go one switch stale and this fails.
  it('resolvedLanguage is already current when languageChanged fires', async () => {
    const seen: Array<string | undefined> = []
    const listener = () => seen.push(i18n.resolvedLanguage)
    i18n.on('languageChanged', listener)
    try {
      await changeAppLanguage('de')
    } finally {
      i18n.off('languageChanged', listener)
    }
    expect(seen[seen.length - 1]).toBe('de')
  })

  // A lazy catalog chunk that fails to load leaves i18next reporting the
  // REQUESTED language while it renders the fallback one. The attribute has to
  // name what is on screen: lang="de" over English text misleads screen readers
  // and language-sniffing crawlers alike.
  it('reports the resolved language, not the requested one', async () => {
    await changeAppLanguage('en')
    const original = i18n.resolvedLanguage
    i18n.resolvedLanguage = 'en'
    try {
      i18n.emit('languageChanged', 'de')
      expect(document.documentElement).toHaveAttribute('lang', 'en')
    } finally {
      i18n.resolvedLanguage = original
    }
  })
})
