import { afterAll, describe, expect, it } from 'vitest'
import i18n from 'i18next'
import { SUPPORTED_LANGUAGE_CODES } from './config'
import { MAX_EXPECTED_SPEAKERS } from '@/components/transcription/ExpectedSpeakersSelector'

type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue }

const modules = import.meta.glob<Record<string, JsonValue>>('./locales/*/transcription.json', {
  eager: true,
  import: 'default',
})

function speakerCountForms(locale: string): Record<string, unknown> {
  const catalog = modules[`./locales/${locale}/transcription.json`]
  expect(catalog, `${locale}/transcription.json is missing`).toBeDefined()
  const section = (catalog as Record<string, Record<string, unknown>>).expectedSpeakers
  expect(section, `${locale} expectedSpeakers block is missing`).toBeDefined()
  return section
}

describe('speaker count plural forms', () => {
  afterAll(async () => {
    await i18n.changeLanguage('en')
  })

  // A language whose catalog is missing a plural category renders the wrong form
  // for every count that falls into it — Russian shipped only one/other, so
  // every count from 2 upwards read wrong. Only the counts the picker can
  // actually produce matter; categories that need millions do not.
  it('covers every plural category the picker can produce', () => {
    const counts = Array.from({ length: MAX_EXPECTED_SPEAKERS }, (_, index) => index + 1)

    for (const locale of SUPPORTED_LANGUAGE_CODES) {
      const forms = speakerCountForms(locale)
      const rules = new Intl.PluralRules(locale)
      for (const count of counts) {
        expect(
          forms,
          `${locale} expectedSpeakers.count_${rules.select(count)} (count=${count})`
        ).toHaveProperty(`count_${rules.select(count)}`)
      }
    }
  })

  it('renders the Russian singular, few and many forms', async () => {
    await i18n.changeLanguage('ru')
    const t = i18n.getFixedT('ru', 'transcription')

    expect(t('expectedSpeakers.count', { count: 1 })).toBe('1 говорящий')
    expect(t('expectedSpeakers.count', { count: 2 })).toBe('2 говорящих')
    expect(t('expectedSpeakers.count', { count: 4 })).toBe('4 говорящих')
    expect(t('expectedSpeakers.count', { count: 5 })).toBe('5 говорящих')
    expect(t('expectedSpeakers.count', { count: 21 })).toBe('21 говорящий')
  })

  it('renders the English singular and plural', async () => {
    await i18n.changeLanguage('en')
    const t = i18n.getFixedT('en', 'transcription')

    expect(t('expectedSpeakers.count', { count: 1 })).toBe('1 speaker')
    expect(t('expectedSpeakers.count', { count: 3 })).toBe('3 speakers')
  })
})
