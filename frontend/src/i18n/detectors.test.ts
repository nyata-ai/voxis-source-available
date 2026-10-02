import { toSupportedLocale, preferredNavigatorDetector, timezoneDetector } from './detectors'
import { withTimeZone } from './test-helpers'

/**
 * Replace navigator.languages for the duration of fn.
 *
 * navigator.languages is a prototype accessor in jsdom, so defineProperty here
 * shadows it with an instance property. `delete` is what restores it — a saved
 * descriptor would be undefined, and the shadow would leak to later tests.
 */
function withRawNavigatorLanguages(value: unknown, fn: () => void) {
  Object.defineProperty(navigator, 'languages', { value, configurable: true })
  try {
    fn()
  } finally {
    delete (navigator as { languages?: unknown }).languages
  }
}

function withNavigatorLanguages(langs: string[], fn: () => void) {
  withRawNavigatorLanguages(langs, fn)
}

describe('toSupportedLocale', () => {
  it('passes through an exact supported code', () => {
    expect(toSupportedLocale('de')).toBe('de')
    expect(toSupportedLocale('zh-CN')).toBe('zh-CN')
  })

  // The whole point: i18next matches supportedLngs EXACTLY on its first pass,
  // so a regional variant must be reduced to a shipped code before it is
  // returned, or a later detector's exact code wins instead.
  it('reduces a regional variant to its base language', () => {
    expect(toSupportedLocale('de-DE')).toBe('de')
    expect(toSupportedLocale('de-CH')).toBe('de')
    expect(toSupportedLocale('ja-JP')).toBe('ja')
    expect(toSupportedLocale('en-US')).toBe('en')
  })

  it('expands a bare language to the shipped regional code', () => {
    expect(toSupportedLocale('zh')).toBe('zh-CN')
  })

  // Documents what actually happens to Traditional Chinese readers. Voxis ships
  // no Traditional catalog, so every zh tag collapses onto Simplified — the same
  // as i18next's own base-language fallback did before geo detection existed.
  // Intentional and unchanged: Simplified is closer for these readers than
  // English, and shipping a Traditional catalog is a product decision, not a fix.
  it('collapses Traditional Chinese tags onto Simplified', () => {
    expect(toSupportedLocale('zh-TW')).toBe('zh-CN')
    expect(toSupportedLocale('zh-Hant')).toBe('zh-CN')
    expect(toSupportedLocale('zh-HK')).toBe('zh-CN')
    expect(toSupportedLocale('zh-MO')).toBe('zh-CN')
  })

  it('is case-insensitive', () => {
    expect(toSupportedLocale('zh-cn')).toBe('zh-CN')
    expect(toSupportedLocale('DE')).toBe('de')
  })

  it('returns undefined for unsupported or empty input', () => {
    expect(toSupportedLocale('pt-BR')).toBeUndefined()
    expect(toSupportedLocale('xx')).toBeUndefined()
    expect(toSupportedLocale('')).toBeUndefined()
  })
})

describe('preferredNavigatorDetector', () => {
  const lookup = () => preferredNavigatorDetector.lookup({})

  // An all-English list is what every OS ships by default, so inside a target
  // market it carries no information about what the reader wants. This is the
  // case the whole feature exists for.
  it('ignores an all-English list inside a target market so geo can decide', () => {
    withTimeZone('Asia/Jakarta', () =>
      withNavigatorLanguages(['en-US', 'en'], () => expect(lookup()).toBeUndefined()),
    )
  })

  it('returns a normalised non-English preference', () => {
    withNavigatorLanguages(['de-DE'], () => expect(lookup()).toBe('de'))
    withNavigatorLanguages(['ja-JP', 'en-US'], () => expect(lookup()).toBe('ja'))
    withNavigatorLanguages(['zh'], () => expect(lookup()).toBe('zh-CN'))
  })

  // A mixed list is a real choice, so the first supported entry wins — English
  // included. Unsupported entries are still skipped.
  it('returns the first supported entry of a mixed list, English included', () => {
    withNavigatorLanguages(['en-GB', 'pt-BR', 'ko'], () => expect(lookup()).toBe('en'))
    withNavigatorLanguages(['ja-JP', 'en-US'], () => expect(lookup()).toBe('ja'))
    withNavigatorLanguages(['pt-BR', 'ko'], () => expect(lookup()).toBe('ko'))
  })

  // Regression: skipping every English entry handed the entire app to a
  // secondary language. A US reader who added Spanish for web content still
  // wants the software in English.
  it('keeps English when Spanish is only a secondary language', () => {
    withNavigatorLanguages(['en-US', 'es-419'], () => expect(lookup()).toBe('en'))
  })

  it('returns undefined when no entry is supported', () => {
    withNavigatorLanguages(['pt-BR', 'sw'], () => expect(lookup()).toBeUndefined())
  })

  // jsdom's navigator.language is 'en-US'. Asserting 'en' rather than undefined
  // is what makes this cover the fallback branch: had navigator.language not
  // been read, the code list would be empty and the answer would be undefined.
  // (The suite pins TZ=UTC, which is unmapped, so English does not yield here.)
  it('falls back to navigator.language when the list is empty', () => {
    withNavigatorLanguages([], () => expect(lookup()).toBe('en'))
  })
})

/**
 * The precedence rule, which is the one place the two detectors are coupled.
 *
 * English first in a mixed list is normally a real choice, but inside a target
 * market the timezone is the better signal: an Indonesian professional on
 * English-configured Windows is the case this feature exists for, whether or not
 * they also listed Indonesian. English yields to geo there, and only there.
 */
describe('preferredNavigatorDetector precedence against the timezone', () => {
  const lookup = () => preferredNavigatorDetector.lookup({})

  it.each<[string[], string, string | undefined]>([
    [['en-US', 'id'], 'Asia/Jakarta', undefined],
    [['en-US', 'id'], 'America/New_York', 'en'],
    [['en-US', 'es-419'], 'America/New_York', 'en'],
    [['en-US', 'es-419'], 'Asia/Jakarta', undefined],
    [['de-DE', 'id'], 'Asia/Jakarta', 'de'],
    [['ja-JP', 'en-US'], 'Europe/Berlin', 'ja'],
    [['en-US', 'en'], 'Asia/Jakarta', undefined],
    [['en-US', 'en'], 'America/New_York', 'en'],
  ])('%j in %s yields %s', (langs, zone, expected) => {
    withTimeZone(zone, () => withNavigatorLanguages(langs, () => expect(lookup()).toBe(expected)))
  })
})

/**
 * navigator.languages is typed `readonly string[]`, but anti-fingerprinting
 * extensions and some embedded webviews really do substitute other shapes.
 *
 * A throw from this lookup is fatal rather than degraded: detection runs
 * synchronously inside i18next's init(), which main.tsx imports at module scope
 * before createRoot().render(), so the exception escapes with no React error
 * boundary mounted and the visitor gets a blank page.
 */
describe('preferredNavigatorDetector with a malformed navigator.languages', () => {
  const lookup = () => preferredNavigatorDetector.lookup({})

  it.each([
    ['a bare string', 'en-US'],
    ['a list holding a number', [5, 'id']],
    ['a list holding null', [null, 'de-DE']],
    ['null', null],
    ['a number', 42],
    ['an object', { 0: 'en-US', length: 1 }],
  ])('does not throw when navigator.languages is %s', (_label, value) => {
    withRawNavigatorLanguages(value, () => expect(lookup).not.toThrow())
  })

  it('does not throw when reading navigator.languages throws', () => {
    Object.defineProperty(navigator, 'languages', {
      get() {
        throw new Error('blocked by extension')
      },
      configurable: true,
    })
    try {
      expect(lookup).not.toThrow()
    } finally {
      delete (navigator as { languages?: unknown }).languages
    }
  })

  // A list that is not a list carries no usable preference. Reporting nothing is
  // free: i18next's own `navigator` detector still runs last in `order` and reads
  // the same properties, so a truthful navigator.language is outranked, not lost.
  it('treats a non-array navigator.languages as no signal', () => {
    withRawNavigatorLanguages('en-US', () => expect(lookup()).toBeUndefined())
    withRawNavigatorLanguages(null, () => expect(lookup()).toBeUndefined())
    withRawNavigatorLanguages(42, () => expect(lookup()).toBeUndefined())
  })

  it('skips non-string entries instead of reading them', () => {
    withRawNavigatorLanguages([5], () => expect(lookup()).toBeUndefined())
    withRawNavigatorLanguages([null], () => expect(lookup()).toBeUndefined())
    withRawNavigatorLanguages([undefined, {}], () => expect(lookup()).toBeUndefined())
  })

  it('still honours the real tags around a malformed entry', () => {
    withRawNavigatorLanguages([5, 'id'], () => expect(lookup()).toBe('id'))
    withRawNavigatorLanguages([null, 'de-DE'], () => expect(lookup()).toBe('de'))
  })
})

describe('timezoneDetector', () => {
  it('returns the mapped locale', () => {
    withTimeZone('Asia/Jakarta', () => expect(timezoneDetector.lookup({})).toBe('id'))
  })

  it('returns undefined for an unmapped zone', () => {
    withTimeZone('America/New_York', () => expect(timezoneDetector.lookup({})).toBeUndefined())
  })
})
