import enRaw from './content/guide-en.md?raw'
import idRaw from './content/guide-id.md?raw'
import deRaw from './content/guide-de.md?raw'
import zhCNRaw from './content/guide-zh-CN.md?raw'

export type GuideBlock =
  | { kind: 'p'; text: string }
  | { kind: 'ol'; start: number; items: string[] }
  | { kind: 'ul'; items: string[] }
  | { kind: 'screenshot'; name: string; alt: string }
export interface GuideChapter {
  slug: string
  title: string
  blocks: GuideBlock[]
}
const guides: Record<string, string> = { en: enRaw, id: idRaw, de: deRaw, 'zh-CN': zhCNRaw }
const cache = new Map<string, GuideChapter[]>()

export function getGuideChapters(language: string): GuideChapter[] {
  const locale = guides[language] ? language : 'en'
  const cached = cache.get(locale)
  if (cached) return cached
  const chapters: GuideChapter[] = []
  const parts = guides[locale].split(/^## \{#([a-z0-9-]+)\} (.+)$/m)
  for (let index = 1; index + 2 < parts.length; index += 3) {
    chapters.push({
      slug: parts[index],
      title: parts[index + 1].trim(),
      blocks: parseBlocks(parts[index + 2]),
    })
  }
  cache.set(locale, chapters)
  return chapters
}

function parseBlocks(body: string): GuideBlock[] {
  const blocks: GuideBlock[] = []
  let list: Extract<GuideBlock, { kind: 'ol' | 'ul' }> | null = null
  const flush = () => {
    if (list) blocks.push(list)
    list = null
  }
  for (const line of body.split(/\r?\n/).map((value) => value.trim())) {
    if (!line) {
      flush()
      continue
    }
    const screenshot = line.match(/^\[\[screenshot:([a-z0-9-]+)\|(.+)\]\]$/)
    if (screenshot) {
      flush()
      blocks.push({ kind: 'screenshot', name: screenshot[1], alt: screenshot[2] })
      continue
    }
    const ordered = line.match(/^(\d+)\.\s+(.*)$/)
    if (ordered) {
      if (!list || list.kind !== 'ol') {
        flush()
        list = { kind: 'ol', start: Number(ordered[1]), items: [] }
      }
      list.items.push(ordered[2])
      continue
    }
    const unordered = line.match(/^[-*]\s+(.*)$/)
    if (unordered) {
      if (!list || list.kind !== 'ul') {
        flush()
        list = { kind: 'ul', items: [] }
      }
      list.items.push(unordered[1])
      continue
    }
    flush()
    blocks.push({ kind: 'p', text: line })
  }
  flush()
  return blocks
}
