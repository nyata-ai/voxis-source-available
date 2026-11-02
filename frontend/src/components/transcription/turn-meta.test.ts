import { describe, expect, it } from 'vitest'
import i18n from '@/i18n'
import type { Utterance } from '@/types/transcription'
import { buildTurnMeta, findTurnAt, htmlLang, languageKey } from './turn-meta'

function turn(speaker: number, start: number, end: number, language?: string): Utterance {
  return { id: start, speaker, start, end, text: 'x', words: [], language }
}

const t = i18n.getFixedT('en')

describe('buildTurnMeta', () => {
  it('shows the speaker only on the first turn of each run', () => {
    const meta = buildTurnMeta([turn(0, 0, 1), turn(0, 1, 2), turn(1, 2, 3), turn(0, 3, 4)], t, 'en')
    expect(meta.map((m) => m.showSpeaker)).toEqual([true, false, true, true])
  })

  it('labels a turn whose language differs from the turn before, in codes and in words', () => {
    const meta = buildTurnMeta([turn(0, 0, 1, 'id'), turn(1, 1, 2, 'id'), turn(0, 2, 3, 'en')], t, 'en')
    expect(meta[0]!.switchText).toBeUndefined()
    expect(meta[1]!.switchText).toBeUndefined()
    expect(meta[2]!.switchText).toBe('ID → EN')
    expect(meta[2]!.switchLabel).toBe('Detected language changes from Indonesian to English')
  })

  it('claims nothing for a turn without a tag, and compares across it', () => {
    const meta = buildTurnMeta([turn(0, 0, 1, 'id'), turn(0, 1, 2), turn(0, 2, 3, 'en')], t, 'en')
    expect(meta[1]!.switchText).toBeUndefined()
    expect(meta[2]!.switchText).toBe('ID → EN')
  })

  it('treats region variants as the same language', () => {
    const meta = buildTurnMeta([turn(0, 0, 1, 'en-US'), turn(0, 1, 2, 'en')], t, 'en')
    expect(meta[1]!.switchText).toBeUndefined()
  })

  it('shows nothing when the provider tagged no languages at all', () => {
    const meta = buildTurnMeta([turn(0, 0, 1), turn(1, 1, 2)], t, 'en')
    expect(meta.every((m) => m.switchText === undefined)).toBe(true)
  })

  it('reads Melia\'s Chinese varieties as Chinese, with no switch between them', () => {
    const meta = buildTurnMeta(
      [turn(0, 0, 1, 'id'), turn(0, 1, 2, 'cmn'), turn(0, 2, 3, 'yue'), turn(0, 3, 4, 'zh')],
      t,
      'en',
    )
    expect(meta[1]!.switchText).toBe('ID → ZH')
    expect(meta[1]!.switchLabel).toBe('Detected language changes from Indonesian to Chinese')
    expect(meta[2]!.switchText).toBeUndefined()
    expect(meta[3]!.switchText).toBeUndefined()
  })

  it('names a language the app does not offer from the browser, or by its code', () => {
    const meta = buildTurnMeta([turn(0, 0, 1, 'en'), turn(0, 1, 2, 'haw')], t, 'en')
    expect(meta[1]!.switchText).toBe('EN → HAW')
    expect(meta[1]!.switchLabel).toMatch(/^Detected language changes from English to /)
  })
})

describe('languageKey', () => {
  it('reduces a tag to its lower-case primary subtag', () => {
    expect(languageKey('zh-CN')).toBe('zh')
    expect(languageKey(' EN_gb ')).toBe('en')
    expect(languageKey(undefined)).toBe('')
  })

  it('treats cmn and yue as zh', () => {
    expect(languageKey('cmn')).toBe('zh')
    expect(languageKey('YUE')).toBe('zh')
    expect(languageKey('cmn-Hans')).toBe('zh')
  })
})

describe('htmlLang', () => {
  it('gives Chinese varieties the tag the CJK type matches, keeping the script', () => {
    expect(htmlLang('cmn')).toBe('zh')
    expect(htmlLang('yue')).toBe('zh')
    expect(htmlLang('cmn-Hant')).toBe('zh-Hant')
  })

  it('passes any other tag through, and drops a blank one', () => {
    expect(htmlLang('en-US')).toBe('en-US')
    expect(htmlLang('zh-Hant')).toBe('zh-Hant')
    expect(htmlLang('  ')).toBeUndefined()
    expect(htmlLang(undefined)).toBeUndefined()
  })
})

describe('findTurnAt', () => {
  const turns = [turn(0, 0, 2), turn(1, 3, 5), turn(0, 6, 8)]

  it('finds the turn playing at the time', () => {
    expect(findTurnAt(turns, 4)).toBe(1)
  })

  it('lands a time in a pause on the next turn', () => {
    expect(findTurnAt(turns, 2.5)).toBe(1)
  })

  it('lands a time past the end on the last turn, and an empty list nowhere', () => {
    expect(findTurnAt(turns, 99)).toBe(2)
    expect(findTurnAt([], 1)).toBe(-1)
  })
})
