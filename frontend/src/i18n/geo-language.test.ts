import { ZONE_LOCALE, localeFromTimeZone } from './geo-language'
import { withTimeZone } from './test-helpers'

describe('ZONE_LOCALE', () => {
  it('maps every Indonesian zone to id', () => {
    for (const zone of [
      'Asia/Jakarta',
      'Asia/Pontianak',
      'Asia/Makassar',
      'Asia/Ujung_Pandang',
      'Asia/Jayapura',
    ]) {
      expect(ZONE_LOCALE[zone]).toBe('id')
    }
  })

  it('maps every mainland China zone to zh-CN', () => {
    for (const zone of [
      'Asia/Shanghai',
      'Asia/Urumqi',
      'Asia/Chongqing',
      'Asia/Chungking',
      'Asia/Harbin',
      'Asia/Kashgar',
    ]) {
      expect(ZONE_LOCALE[zone]).toBe('zh-CN')
    }
  })

  it('maps every German-speaking zone to de', () => {
    for (const zone of [
      'Europe/Berlin',
      'Europe/Busingen',
      'Europe/Zurich',
      'Europe/Vienna',
      'Europe/Vaduz',
    ]) {
      expect(ZONE_LOCALE[zone]).toBe('de')
    }
  })

  // Traditional-Chinese readers must NOT be handed Simplified; Voxis does not
  // ship zh-TW, so falling through to the browser language is the better answer.
  it('leaves Hong Kong, Macau and Taipei unmapped', () => {
    expect(ZONE_LOCALE['Asia/Hong_Kong']).toBeUndefined()
    expect(ZONE_LOCALE['Asia/Macau']).toBeUndefined()
    expect(ZONE_LOCALE['Asia/Taipei']).toBeUndefined()
  })

  // Not made redundant by ZONE_LOCALE's value type: that only proves the values
  // are locales Voxis ships. This is the narrower product rule — geo speaks for
  // three markets, and nothing else may be mapped without a deliberate decision.
  it('only ever maps to locales Voxis ships', () => {
    for (const locale of Object.values(ZONE_LOCALE)) {
      expect(['id', 'zh-CN', 'de']).toContain(locale)
    }
  })
})

describe('localeFromTimeZone', () => {
  it('resolves a mapped zone', () => {
    withTimeZone('Asia/Jakarta', () => expect(localeFromTimeZone()).toBe('id'))
  })

  it('returns undefined for an unmapped zone', () => {
    withTimeZone('America/New_York', () => expect(localeFromTimeZone()).toBeUndefined())
  })

  // Firefox resistFingerprinting and Tor Browser report UTC. No signal is correct.
  it('returns undefined for UTC', () => {
    withTimeZone('UTC', () => expect(localeFromTimeZone()).toBeUndefined())
  })

  it('returns undefined for an empty or garbage zone', () => {
    withTimeZone('', () => expect(localeFromTimeZone()).toBeUndefined())
    withTimeZone('Not/AZone', () => expect(localeFromTimeZone()).toBeUndefined())
  })

  it('returns undefined instead of throwing when Intl is unavailable', () => {
    withTimeZone(null, () => expect(localeFromTimeZone()).toBeUndefined())
  })
})
