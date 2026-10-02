import { readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

// The web Dockerfile runs `npm ci`, then `COPY . .`, then the build. Anything
// local that reaches the context overrides that clean install or build input.
describe('web image build context', () => {
  const entries = readFileSync(path.resolve(__dirname, '../.dockerignore'), 'utf8')
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith('#'))

  it.each(['node_modules/', 'dist/', '.env', '.env.*'])('excludes %s', (entry) => {
    expect(entries).toContain(entry)
  })

  it('keeps the files the Dockerfile copies before npm ci', () => {
    for (const needed of ['package.json', 'package-lock.json', '.npmrc']) {
      expect(entries).not.toContain(needed)
    }
  })
})
