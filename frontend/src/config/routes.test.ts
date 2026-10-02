import { describe, expect, it } from 'vitest'
import { CAPTURE_TABS, getVisibleNavGroups, routes } from './routes'

describe('Voxis-OSS routes', () => {
  it('exposes only upload and standard recording capture tabs', () => {
    expect(CAPTURE_TABS).toEqual(['upload', 'record'])
  })

  it('does not expose billing, URL transcription, or privilege routes', () => {
    expect(Object.keys(routes)).not.toEqual(expect.arrayContaining(['billing', 'url', 'privilege']))
    expect(Object.values(routes).map((route) => route.path)).not.toEqual(
      expect.arrayContaining(['/billing', '/url-transcriptions', '/privilege'])
    )
  })

  it('shows operational administration only to administrators', () => {
    expect(
      getVisibleNavGroups({ isAdmin: false })
        .flatMap((group) => group.routes)
        .map((route) => route.path)
    ).not.toContain('/admin')
    expect(
      getVisibleNavGroups({ isAdmin: true })
        .flatMap((group) => group.routes)
        .map((route) => route.path)
    ).toContain('/admin')
  })
})
