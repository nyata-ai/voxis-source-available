/**
 * Guard for the app's type system (DESIGN.md, typography).
 *
 * New app work sizes text with the seven named roles in `globals.css`
 * (`text-display` … `text-caption`) and uses two weights, 400 and 600. Plain
 * `text-xs`, `text-sm` and so on remain in older code and are not guarded here.
 * This fails on the three things that crept in before the roles existed:
 *  - arbitrary sizes such as `text-[0.68rem]` or `text-[13px]`;
 *  - `font-medium` (Söhne ships no 500, so it silently renders as 400);
 *  - `font-bold` (700 is above the app's 600 ceiling).
 *
 * The public pages (landing, accuracy, legal, guide) follow their own rules
 * and are exempt.
 */
import { readdirSync, readFileSync } from 'node:fs'
import { join, relative, resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

// Vitest runs from the frontend package root (see lib/accuracy-facts.test.ts).
const SRC = resolve(process.cwd(), 'src')
const ROOTS = ['components', 'pages']
/** App code that lives outside those folders. */
const FILES = ['router.tsx']
const MAX_FILES = 2000

const EXEMPT = [
  /^components\/landing\//,
  /^components\/accuracy\//,
  /^components\/legal\//,
  /^pages\/accuracy\//,
  /^pages\/guide\//,
  /^pages\/LandingPage\./,
  /^pages\/LegalPage\./,
]

const RULES: { name: string; pattern: RegExp }[] = [
  { name: 'arbitrary text size', pattern: /\btext-\[\d*\.?\d+(?:rem|px)\]/g },
  { name: 'font-medium', pattern: /\bfont-medium\b/g },
  { name: 'font-bold', pattern: /\bfont-bold\b/g },
]

function listSources(): string[] {
  const out = FILES.map((file) => join(SRC, file))
  const queue = ROOTS.map((root) => join(SRC, root))
  while (queue.length > 0 && out.length < MAX_FILES) {
    const dir = queue.shift()!
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const full = join(dir, entry.name)
      if (entry.isDirectory()) queue.push(full)
      else if (/\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name)) out.push(full)
    }
  }
  return out
}

function appSources(): string[] {
  return listSources()
    .map((file) => relative(SRC, file).replace(/\\/g, '/'))
    .filter((file) => !EXEMPT.some((rule) => rule.test(file)))
}

describe('app type roles', () => {
  const files = appSources()

  it('scans the app source tree', () => {
    // A path mistake would otherwise make every rule below pass vacuously.
    expect(files.length).toBeGreaterThan(100)
    expect(files).toContain('components/transcription/TranscriptViewer.tsx')
    expect(files).toContain('router.tsx')
    expect(files).not.toContain('pages/LandingPage.tsx')
  })

  for (const rule of RULES) {
    it(`uses no ${rule.name} in app code`, () => {
      const hits: string[] = []
      for (const file of files) {
        const lines = readFileSync(join(SRC, file), 'utf8').split('\n')
        lines.forEach((line, index) => {
          for (const match of line.matchAll(rule.pattern)) {
            hits.push(`${file}:${index + 1} ${match[0]}`)
          }
        })
      }
      expect(hits).toEqual([])
    })
  }
})
