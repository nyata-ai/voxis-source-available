import { afterEach, describe, expect, it } from 'vitest'
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

  describe('in a zone away from UTC', () => {
    // The suite pins TZ=UTC, which hid this. Node re-reads the zone when TZ is
    // assigned, and these tests run in a forked process, so it is theirs to set.
    const original = process.env.TZ
    afterEach(() => {
      process.env.TZ = original
    })

    it('runs in the zone it asks for', () => {
      process.env.TZ = 'Asia/Jakarta'
      expect(new Date('2026-04-13T00:00:00Z').getHours()).toBe(7)
    })

    it('says "today" for this morning in Jakarta, when UTC still says yesterday', () => {
      process.env.TZ = 'Asia/Jakarta'
      const formatters = createFormatters('en')
      // 08:00 WIB on 13 April, a session finished at 06:30 WIB the same morning.
      const now = new Date('2026-04-13T01:00:00Z')
      const finished = new Date('2026-04-12T23:30:00Z')
      expect(formatters.relativeDate(finished, now)).toBe('today')
    })

    it('says "yesterday" across local midnight, when UTC still says today', () => {
      process.env.TZ = 'Asia/Jakarta'
      const formatters = createFormatters('en')
      // Monday 06:00 WIB; the session finished Sunday 20:00 WIB.
      const now = new Date('2026-04-12T23:00:00Z')
      const finished = new Date('2026-04-12T13:00:00Z')
      expect(formatters.relativeDate(finished, now)).toBe('yesterday')
    })

    it('titles the Desk with the local day', () => {
      process.env.TZ = 'Asia/Jakarta'
      const formatters = createFormatters('en')
      expect(formatters.longDate(new Date('2026-04-12T23:00:00Z'))).toBe('Monday, April 13, 2026')
    })

    it('counts calendar days across a daylight-saving change', () => {
      process.env.TZ = 'America/New_York'
      const formatters = createFormatters('en')
      // Clocks went forward on 8 March 2026; that local day is 23 hours long.
      const now = new Date('2026-03-09T12:00:00-04:00')
      expect(formatters.relativeDate(new Date('2026-03-08T00:30:00-05:00'), now)).toBe('yesterday')
    })

    it('keeps a date-only legal date on its own day west of Greenwich', () => {
      process.env.TZ = 'America/New_York'
      const formatters = createFormatters('en')
      expect(formatters.longDateUTC('2026-09-03')).toBe('Thursday, September 3, 2026')
    })
  })

  it('formats UTC month ranges with localized month names', () => {
    const formatters = createFormatters('ja')
    const now = new Date(Date.UTC(2026, 0, 15, 12, 0, 0))

    expect(formatters.monthRangeUTC(now)).toBe('1月 1–15')
  })
})
