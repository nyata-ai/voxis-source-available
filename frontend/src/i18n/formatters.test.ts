import { describe, expect, it } from 'vitest'
import { createFormatters } from './formatters'

describe('createFormatters', () => {
  it('formats numbers with the requested locale', () => {
    const en = createFormatters('en')
    const de = createFormatters('de')

    expect(en.number(1234567.8)).toBe('1,234,567.8')
    expect(de.number(1234567.8)).toBe('1.234.567,8')
  })

  it('formats compact numbers through Intl instead of manual K/M suffixes', () => {
    const id = createFormatters('id')

    expect(id.compactNumber(135000)).toBe(
      new Intl.NumberFormat('id', {
        notation: 'compact',
        maximumFractionDigits: 1,
      }).format(135000)
    )
    expect(id.compactNumber(135000)).not.toBe('135.0K')
  })

  it('formats long dates with the requested locale', () => {
    const date = new Date(Date.UTC(2026, 4, 22, 12, 0, 0))

    expect(createFormatters('fr').longDate(date)).toBe(
      new Intl.DateTimeFormat('fr', {
        weekday: 'long',
        day: 'numeric',
        month: 'long',
        year: 'numeric',
        timeZone: 'UTC',
      }).format(date)
    )
  })

  it('returns an empty string for invalid dates', () => {
    const formatters = createFormatters('en')

    expect(formatters.date('not-a-date')).toBe('')
    expect(formatters.dateTime('not-a-date')).toBe('')
    expect(formatters.relativeDate('not-a-date')).toBe('')
  })

  it('formats relative days with Intl.RelativeTimeFormat', () => {
    const formatters = createFormatters('es')
    const now = new Date(Date.UTC(2026, 5, 7, 12, 0, 0))
    const yesterday = new Date(Date.UTC(2026, 5, 6, 12, 0, 0))

    expect(formatters.relativeDate(yesterday, now)).toBe(
      new Intl.RelativeTimeFormat('es', { numeric: 'auto' }).format(-1, 'day')
    )
  })

  it('formats UTC month ranges with localized month names', () => {
    const formatters = createFormatters('ja')
    const now = new Date(Date.UTC(2026, 0, 15, 12, 0, 0))

    expect(formatters.monthRangeUTC(now)).toBe('1月 1–15')
  })
})
