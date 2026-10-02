import { describe, expect, it } from 'vitest'
import { NAMESPACES, SUPPORTED_LANGUAGE_CODES } from './config'

type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue }

const modules = import.meta.glob<JsonValue>('./locales/*/*.json', {
  eager: true,
  import: 'default',
})

function readNamespace(locale: string, namespace: string): JsonValue {
  const module = modules[`./locales/${locale}/${namespace}.json`]
  expect(module, `${locale}/${namespace}.json is missing`).toBeDefined()
  return module
}

function flatten(value: JsonValue, prefix = '', out = new Map<string, string>()) {
  if (typeof value === 'string') {
    out.set(prefix, value)
    return out
  }

  if (value && typeof value === 'object' && !Array.isArray(value)) {
    for (const [key, child] of Object.entries(value)) {
      flatten(child, prefix ? `${prefix}.${key}` : key, out)
    }
  }

  return out
}

function placeholders(value: string): string[] {
  return (value.match(/{{\s*[^}]+\s*}}/g) ?? []).sort()
}

/**
 * A plural key's family, e.g. "count_one" and "count_few" both belong to
 * "count_*". Key-for-key equality with English is wrong for plurals: how many
 * forms a family needs is a property of the language (Russian needs few/many,
 * English does not), so the families must match while the forms may differ.
 */
const PLURAL_SUFFIX = /_(zero|one|two|few|many|other)$/

function family(key: string): string {
  return key.replace(PLURAL_SUFFIX, '_*')
}

function families(keys: string[]): string[] {
  return [...new Set(keys.map(family))].sort()
}

/** The English string a translated key must match placeholders with. */
function englishCounterpart(english: Map<string, string>, key: string): string | undefined {
  const base = key.replace(PLURAL_SUFFIX, '')
  return english.get(key) ?? english.get(`${base}_other`) ?? english.get(`${base}_one`)
}

describe('i18n locale catalogs', () => {
  it('ships complete catalogs for every supported UI language', () => {
    for (const namespace of NAMESPACES) {
      const english = flatten(readNamespace('en', namespace))
      const englishFamilies = families([...english.keys()])

      for (const locale of SUPPORTED_LANGUAGE_CODES) {
        const translated = flatten(readNamespace(locale, namespace))

        expect(families([...translated.keys()]), `${locale}/${namespace}.json keys`).toEqual(
          englishFamilies
        )

        for (const [key, value] of translated) {
          expect(value.trim(), `${locale}/${namespace}.${key} is empty`).not.toBe('')
          const counterpart = englishCounterpart(english, key)
          expect(counterpart, `${locale}/${namespace}.${key} has no English counterpart`).toBeDefined()
          expect(placeholders(value), `${locale}/${namespace}.${key} placeholders`).toEqual(
            placeholders(counterpart ?? '')
          )
        }
      }
    }
  })

  it('names the product "Voxis Source-Available", never the old repository name', () => {
    for (const [path, catalog] of Object.entries(modules)) {
      for (const [key, value] of flatten(catalog)) {
        expect(value, `${path} ${key}`).not.toMatch(/Voxis[- ]OSS|nyata-ai\/Voxis-OSS/i)
      }
    }
  })
})
