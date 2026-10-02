import { describe, it, expect } from 'vitest'
import { cn, formatDuration, formatBytes, formatIDR } from './utils'

describe('formatIDR', () => {
  // id-ID renders a non-breaking space after the symbol; normalize for assertions.
  const plain = (value: number) => formatIDR(value).replace(/\u00a0/g, ' ')

  it('formats whole rupiah amounts with dot grouping', () => {
    expect(plain(85_000)).toBe('Rp 85.000')
    expect(plain(1_700_000)).toBe('Rp 1.700.000')
  })

  it('drops fractions', () => {
    expect(plain(17_000.6)).toBe('Rp 17.001')
    expect(plain(0)).toBe('Rp 0')
  })

  it('formats negative amounts', () => {
    expect(plain(-17_000)).toBe('-Rp 17.000')
  })

  it('falls back to zero for non-finite input', () => {
    expect(plain(Number.NaN)).toBe('Rp 0')
    expect(plain(Number.POSITIVE_INFINITY)).toBe('Rp 0')
  })
})

describe('cn (class name utility)', () => {
  it('merges class names', () => {
    expect(cn('foo', 'bar')).toBe('foo bar')
  })

  it('handles conditional classes', () => {
    const condition = false
    expect(cn('foo', condition && 'bar', 'baz')).toBe('foo baz')
  })

  it('handles undefined and null', () => {
    expect(cn('foo', undefined, null, 'bar')).toBe('foo bar')
  })

  it('handles Tailwind conflicts - last wins', () => {
    // twMerge should resolve conflicts, keeping the last one
    expect(cn('p-4', 'p-2')).toBe('p-2')
    expect(cn('text-red-500', 'text-blue-500')).toBe('text-blue-500')
  })

  it('handles array of classes', () => {
    expect(cn(['foo', 'bar'])).toBe('foo bar')
  })

  it('handles object syntax', () => {
    expect(cn({ foo: true, bar: false, baz: true })).toBe('foo baz')
  })

  it('handles empty input', () => {
    expect(cn()).toBe('')
    expect(cn('')).toBe('')
  })

  it('handles mixed inputs', () => {
    expect(cn('base', { active: true }, ['extra'], undefined)).toBe('base active extra')
  })
})

describe('formatDuration', () => {
  it('formats seconds to mm:ss', () => {
    expect(formatDuration(65)).toBe('01:05')
  })

  it('formats hours correctly', () => {
    expect(formatDuration(3661)).toBe('01:01:01')
  })

  it('handles zero', () => {
    expect(formatDuration(0)).toBe('00:00')
  })

  it('handles negative values', () => {
    expect(formatDuration(-1)).toBe('00:00')
    expect(formatDuration(-100)).toBe('00:00')
  })

  it('formats exact minutes', () => {
    expect(formatDuration(60)).toBe('01:00')
    expect(formatDuration(120)).toBe('02:00')
  })

  it('formats exact hours', () => {
    expect(formatDuration(3600)).toBe('01:00:00')
    expect(formatDuration(7200)).toBe('02:00:00')
  })

  it('handles large values', () => {
    expect(formatDuration(86400)).toBe('24:00:00') // 24 hours
    expect(formatDuration(90061)).toBe('25:01:01') // 25 hours, 1 minute, 1 second
  })

  it('pads single digits correctly', () => {
    expect(formatDuration(1)).toBe('00:01')
    expect(formatDuration(9)).toBe('00:09')
    expect(formatDuration(61)).toBe('01:01')
  })
})

describe('formatBytes', () => {
  it('formats bytes', () => {
    expect(formatBytes(500)).toBe('500 B')
  })

  it('formats kilobytes', () => {
    expect(formatBytes(1536)).toBe('1.5 KB')
  })

  it('formats megabytes', () => {
    expect(formatBytes(1572864)).toBe('1.5 MB')
  })

  it('handles zero', () => {
    expect(formatBytes(0)).toBe('0 B')
  })

  it('handles negative values', () => {
    expect(formatBytes(-1)).toBe('0 B')
    expect(formatBytes(-100)).toBe('0 B')
  })

  it('formats gigabytes', () => {
    expect(formatBytes(1610612736)).toBe('1.5 GB')
  })

  it('formats terabytes', () => {
    expect(formatBytes(1649267441664)).toBe('1.5 TB')
  })

  it('respects decimals parameter', () => {
    expect(formatBytes(1536, 0)).toBe('2 KB')
    expect(formatBytes(1536, 1)).toBe('1.5 KB')
    expect(formatBytes(1536, 3)).toBe('1.5 KB')
  })

  it('handles exact powers of 1024', () => {
    expect(formatBytes(1024)).toBe('1 KB')
    expect(formatBytes(1048576)).toBe('1 MB')
    expect(formatBytes(1073741824)).toBe('1 GB')
  })
})
