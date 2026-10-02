import { describe, expect, it } from 'vitest'
import { getGuideChapters } from './guide-content'

const expectedScreenshots = ['dashboard-capture', 'library-search', 'activity-failure', 'record', 'reader', 'briefings']

function screenshotNames(language: string) {
  return getGuideChapters(language).flatMap((chapter) =>
    chapter.blocks.filter((block) => block.kind === 'screenshot').map((block) => block.name)
  )
}

describe('Voxis Source-Available guide content', () => {
  it('keeps the four guides structurally aligned', () => {
    const english = getGuideChapters('en').map((chapter) => chapter.slug)
    expect(english).toEqual(['welcome', 'sign-in', 'upload', 'record', 'summaries', 'admin'])
    for (const language of ['id', 'de', 'zh-CN'])
      expect(getGuideChapters(language).map((chapter) => chapter.slug)).toEqual(english)
  })

  it('uses the same captured Voxis Source-Available screens in every guide', () => {
    for (const language of ['en', 'id', 'de', 'zh-CN'])
      expect(screenshotNames(language)).toEqual(expectedScreenshots)
  })

  it('states the Speechmatics boundary and PDF script limitation in English', () => {
    const text = getGuideChapters('en')
      .flatMap((chapter) => chapter.blocks)
      .map((block) =>
        block.kind === 'p'
          ? block.text
          : block.kind === 'screenshot'
            ? block.alt
            : block.items.join(' ')
      )
      .join(' ')
    expect(text).toContain('Speechmatics')
    expect(text).toContain(
      'PDF export does not reliably support Chinese, Japanese, or Korean scripts'
    )
  })
})
