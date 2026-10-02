import { describe, it, expect } from 'vitest'
import { generateBars, hashString, mulberry32 } from './waveform-seed'

describe('hashString', () => {
  it('produces consistent output for the same string', () => {
    expect(hashString('voxis')).toBe(hashString('voxis'))
  })

  it('produces different output for different strings', () => {
    expect(hashString('voxis')).not.toBe(hashString('voxis '))
  })

  it('handles empty string without throwing', () => {
    expect(() => hashString('')).not.toThrow()
    expect(typeof hashString('')).toBe('number')
  })

  it('returns a non-negative 32-bit integer', () => {
    const h = hashString('any-string-12345')
    expect(h).toBeGreaterThanOrEqual(0)
    expect(h).toBeLessThan(2 ** 32)
    expect(Number.isInteger(h)).toBe(true)
  })
})

describe('mulberry32', () => {
  it('is deterministic for the same seed', () => {
    const a = mulberry32(42)
    const b = mulberry32(42)
    for (let i = 0; i < 5; i++) {
      expect(a()).toBe(b())
    }
  })

  it('handles seed 0 without producing a degenerate stream', () => {
    const rng = mulberry32(0)
    const samples = Array.from({ length: 10 }, () => rng())
    // Not all identical, no NaNs.
    expect(new Set(samples).size).toBeGreaterThan(1)
    samples.forEach((s) => {
      expect(Number.isFinite(s)).toBe(true)
      expect(s).toBeGreaterThanOrEqual(0)
      expect(s).toBeLessThan(1)
    })
  })

  it('returns values in [0, 1)', () => {
    const rng = mulberry32(0xdeadbeef)
    for (let i = 0; i < 100; i++) {
      const v = rng()
      expect(v).toBeGreaterThanOrEqual(0)
      expect(v).toBeLessThan(1)
    }
  })
})

describe('generateBars', () => {
  it('is deterministic — same seed produces identical bars', () => {
    const a = generateBars('seed-x', 60, { shape: 'hero' })
    const b = generateBars('seed-x', 60, { shape: 'hero' })
    expect(a).toEqual(b)
  })

  it('different seeds produce different bars', () => {
    const a = generateBars('seed-a', 60, { shape: 'hero' })
    const b = generateBars('seed-b', 60, { shape: 'hero' })
    expect(a).not.toEqual(b)
  })

  it('different salts produce different bars even with same seed', () => {
    const a = generateBars('shared', 60, { shape: 'hero', saltA: 0 })
    const b = generateBars('shared', 60, { shape: 'hero', saltA: 7 })
    expect(a).not.toEqual(b)
  })

  it('respects requested count', () => {
    expect(generateBars('seed', 16, { shape: 'mini' }).length).toBe(16)
    expect(generateBars('seed', 60, { shape: 'hero' }).length).toBe(60)
  })

  it('produces all values clamped to the configured floor and ceiling', () => {
    const bars = generateBars('any', 60, { shape: 'hero' })
    bars.forEach((b) => {
      expect(b).toBeGreaterThanOrEqual(0.08) // MIN_AMPLITUDE_HERO
      expect(b).toBeLessThanOrEqual(1)
    })
  })

  it('mini shape has a higher noise floor than hero', () => {
    const mini = generateBars('any', 16, { shape: 'mini' })
    mini.forEach((b) => {
      expect(b).toBeGreaterThanOrEqual(0.15) // MIN_AMPLITUDE_MINI
    })
  })

  it('handles count of 1 without division-by-zero', () => {
    const result = generateBars('seed', 1, { shape: 'hero' })
    expect(result.length).toBe(1)
    expect(Number.isFinite(result[0])).toBe(true)
  })

  it('does not produce NaN for empty seed', () => {
    const bars = generateBars('', 16, { shape: 'mini' })
    bars.forEach((b) => expect(Number.isFinite(b)).toBe(true))
  })
})
