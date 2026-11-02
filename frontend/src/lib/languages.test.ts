/**
 * Drift guard for the numbers the landing page states about the language list.
 *
 * `comparison.products.voxis.languages` in the en, id, de and zh-CN landing
 * catalogs (`src/i18n/locales/<locale>/landing.json`) says "26 languages" and "up
 * to five" in words, and the comparison table and the search engines read those
 * strings. They cannot follow this file on their own. If this test fails because
 * the list changed, change those four strings in the same commit, then the numbers
 * here. The es, fr, ja, ko and ru catalogs state only the "five".
 */
import { describe, expect, it } from 'vitest'
import { LANGUAGES, MAX_LANGUAGES } from './languages'

describe('the language counts the landing page states', () => {
  it('offers 26 languages', () => {
    expect(LANGUAGES).toHaveLength(26)
  })

  it('mixes up to five in one conversation', () => {
    expect(MAX_LANGUAGES).toBe(5)
  })
})
