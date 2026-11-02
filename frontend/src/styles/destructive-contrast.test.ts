/**
 * The destructive button (Revoke, Delete) sets `--destructive-foreground` on
 * `--destructive`. In dark mode the fill is a light red, and the old near-white
 * text on it measured 2.47:1.
 */
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

// Vitest runs from the frontend package root (see lib/accuracy-facts.test.ts).
const CSS = readFileSync(resolve(process.cwd(), 'src/styles/globals.css'), 'utf8')

type Rgb = [number, number, number]

/** Every value a token takes in the stylesheet, light first, as sRGB 0-1. */
function tokenValues(name: string): Rgb[] {
  const pattern = new RegExp(`--${name}: (\\d+) (\\d+)% (\\d+)%;`, 'g')
  return [...CSS.matchAll(pattern)].map((match) => hslToRgb(Number(match[1]), Number(match[2]) / 100, Number(match[3]) / 100))
}

function hslToRgb(h: number, s: number, l: number): Rgb {
  const a = s * Math.min(l, 1 - l)
  const channel = (n: number) => {
    const k = (n + h / 30) % 12
    return l - a * Math.max(-1, Math.min(k - 3, 9 - k, 1))
  }
  return [channel(0), channel(8), channel(4)]
}

function luminance([r, g, b]: Rgb): number {
  const linear = (c: number) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4)
  return 0.2126 * linear(r) + 0.7152 * linear(g) + 0.0722 * linear(b)
}

function contrast(a: Rgb, b: Rgb): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x) as [number, number]
  return (hi + 0.05) / (lo + 0.05)
}

/** `hover:bg-destructive/90`: the fill at 90% over the page. */
function over(fill: Rgb, page: Rgb, alpha: number): Rgb {
  return fill.map((c, i) => c * alpha + page[i]! * (1 - alpha)) as Rgb
}

describe('destructive button contrast', () => {
  const fills = tokenValues('destructive')
  const texts = tokenValues('destructive-foreground')
  const pages = tokenValues('background')

  it('finds a light and a dark value for each token', () => {
    expect(fills).toHaveLength(2)
    expect(texts).toHaveLength(2)
    expect(pages.length).toBeGreaterThanOrEqual(2)
  })

  it.each([
    ['light', 0],
    ['dark', 1],
  ])('passes AA in %s mode, at rest and on hover', (_mode, index) => {
    const fill = fills[index]!
    const text = texts[index]!
    expect(contrast(text, fill)).toBeGreaterThanOrEqual(4.5)
    expect(contrast(text, over(fill, pages[index]!, 0.9))).toBeGreaterThanOrEqual(4.5)
  })
})
