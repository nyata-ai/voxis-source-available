import { createInstance } from 'i18next'
import { TRANSLATED_LANGUAGE_CODES, DEFAULT_LOCALE } from './config'

// i18next resolves a detected array by taking the first EXACT member of
// supportedLngs, and only falls back to base-language matching if nothing
// matched exactly. These cases lock that behaviour in: they are what make
// normalising in preferredNavigator mandatory rather than cosmetic.
async function resolve(detected: string[]): Promise<string> {
  const i18n = createInstance()
  await i18n.init({
    lng: DEFAULT_LOCALE,
    supportedLngs: [...TRANSLATED_LANGUAGE_CODES],
    fallbackLng: DEFAULT_LOCALE,
    resources: {},
  })
  return i18n.services.languageUtils.getBestMatchFromCodes(detected) as string
}

describe('i18next detected-array resolution', () => {
  it('takes an exact supported code over an earlier regional variant', async () => {
    // The failure mode this whole design guards against.
    await expect(resolve(['de-DE', 'id'])).resolves.toBe('id')
  })

  it('honours order once the earlier code is normalised', async () => {
    await expect(resolve(['de', 'id'])).resolves.toBe('de')
    await expect(resolve(['ja', 'de'])).resolves.toBe('ja')
    await expect(resolve(['zh-CN', 'de'])).resolves.toBe('zh-CN')
  })

  it('lets geo win over an English browser', async () => {
    await expect(resolve(['id'])).resolves.toBe('id')
  })

  it('falls back to the default when nothing is supported', async () => {
    await expect(resolve(['pt-BR'])).resolves.toBe(DEFAULT_LOCALE)
  })
})
