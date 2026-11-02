import { describe, expect, it } from 'vitest'
import { SPEAKER_PALETTE_SIZE, speakerInitial, speakerTone } from './speaker-tone'

describe('speakerTone', () => {
  it('gives the first five speakers the five fills in palette order', () => {
    expect([0, 1, 2, 3, 4].map((i) => speakerTone(i).fill)).toEqual([
      'bg-peach',
      'bg-mint',
      'bg-lilac',
      'bg-butter',
      'bg-sky',
    ])
    expect([0, 1, 2, 3, 4].every((i) => !speakerTone(i).ringed)).toBe(true)
  })

  it('reuses the fills with an ink ring for speakers 6 to 10, so 1 and 6 still differ', () => {
    expect(SPEAKER_PALETTE_SIZE).toBe(5)
    expect(speakerTone(5)).toEqual({ fill: 'bg-peach', ringed: true })
    expect(speakerTone(9)).toEqual({ fill: 'bg-sky', ringed: true })
    expect(speakerTone(5)).not.toEqual(speakerTone(0))
  })

  it('keeps the ring past ten, so a later speaker never looks like one of the first five', () => {
    expect(speakerTone(10)).toEqual({ fill: 'bg-peach', ringed: true })
    expect(speakerTone(16)).toEqual({ fill: 'bg-mint', ringed: true })
  })

  it('is keyed on the number alone, so it is stable across calls', () => {
    expect(speakerTone(3)).toEqual(speakerTone(3))
  })

  it('falls back to the first tone for a malformed number instead of throwing', () => {
    expect(speakerTone(-1)).toEqual(speakerTone(0))
    expect(speakerTone(Number.NaN)).toEqual(speakerTone(0))
    expect(speakerTone(1.5)).toEqual(speakerTone(0))
  })
})

describe('speakerInitial', () => {
  it('uses the first letter of the name, upper-cased', () => {
    expect(speakerInitial('ana Vidal', 0)).toBe('A')
    expect(speakerInitial('  Tomas ', 1)).toBe('T')
  })

  it('uses the first whole character of a CJK name or an emoji', () => {
    expect(speakerInitial('王小明', 0)).toBe('王')
    expect(speakerInitial('𠮷田', 0)).toBe('𠮷')
  })

  it('shows the one-based speaker number while the speaker is unnamed', () => {
    expect(speakerInitial(undefined, 0)).toBe('1')
    expect(speakerInitial('   ', 6)).toBe('7')
  })
})
