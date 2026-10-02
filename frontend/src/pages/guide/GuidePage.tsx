import { useEffect, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { getGuideChapters, type GuideBlock } from './guide-content'
import dashboardCapture from '@/assets/guide/dashboard-capture.jpg'
import activityFailure from '@/assets/guide/activity-failure.jpg'
import record from '@/assets/guide/record.jpg'
import reader from '@/assets/guide/reader.jpg'
import briefings from '@/assets/guide/briefings.jpg'
import librarySearch from '@/assets/guide/library-search.jpg'

const screenshots: Record<string, string> = {
  'activity-failure': activityFailure,
  'dashboard-capture': dashboardCapture,
  record,
  reader,
  briefings,
  'library-search': librarySearch,
}

function inline(text: string): ReactNode[] {
  const parts = text.split(/(\*\*[^*]+\*\*|`[^`]+`)/g)
  return parts.filter(Boolean).map((part, index) =>
    part.startsWith('**') ? (
      <strong key={index}>{part.slice(2, -2)}</strong>
    ) : part.startsWith('`') ? (
      <code key={index} className="rounded border bg-muted px-1 py-0.5">
        {part.slice(1, -1)}
      </code>
    ) : (
      part
    )
  )
}

function Block({ block }: { block: GuideBlock }) {
  if (block.kind === 'p')
    return <p className="mb-4 leading-relaxed text-foreground/90">{inline(block.text)}</p>
  if (block.kind === 'screenshot')
    return (
      <figure className="mb-6 overflow-hidden rounded-lg border bg-muted/20">
        <img src={screenshots[block.name]} alt={block.alt} className="block h-auto w-full" />
      </figure>
    )
  const List = block.kind === 'ol' ? 'ol' : 'ul'
  return (
    <List
      {...(block.kind === 'ol' ? { start: block.start } : {})}
      className={
        block.kind === 'ol'
          ? 'mb-4 list-decimal space-y-1.5 pl-5 leading-relaxed'
          : 'mb-4 list-disc space-y-1.5 pl-5 leading-relaxed'
      }
    >
      {block.items.map((item, index) => (
        <li key={index}>{inline(item)}</li>
      ))}
    </List>
  )
}

export function GuidePage() {
  const { i18n } = useTranslation()
  const chapters = getGuideChapters(i18n.resolvedLanguage ?? 'en')
  useEffect(() => {
    const previous = document.title
    document.title = 'Voxis Source-Available — User guide'
    return () => {
      document.title = previous
    }
  }, [])
  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="sticky top-0 z-10 border-b bg-background/95 px-6 py-4 backdrop-blur">
        <h1 className="text-lg font-semibold">
          Voxis Source-Available{' '}
          <span className="font-normal text-muted-foreground">User guide</span>
        </h1>
      </header>
      <div className="mx-auto grid max-w-5xl gap-10 px-6 py-10 lg:grid-cols-[14rem_minmax(0,68ch)]">
        <nav aria-label="User guide" className="lg:sticky lg:top-24 lg:h-fit">
          <ol className="space-y-2 text-sm">
            {chapters.map((chapter, index) => (
              <li key={chapter.slug}>
                <a
                  href={`#${chapter.slug}`}
                  className="text-muted-foreground hover:text-foreground"
                >
                  {String(index + 1).padStart(2, '0')} {chapter.title}
                </a>
              </li>
            ))}
          </ol>
        </nav>
        <main className="min-w-0 max-w-[68ch]">
          {chapters.map((chapter, index) => (
            <section
              key={chapter.slug}
              id={chapter.slug}
              className="scroll-mt-20 border-b py-9 last:border-b-0"
            >
              <p className="text-xs font-semibold tracking-widest text-primary">
                {String(index + 1).padStart(2, '0')}
              </p>
              <h2 className="mb-4 mt-1 text-2xl font-semibold">{chapter.title}</h2>
              {chapter.blocks.map((block, blockIndex) => (
                <Block key={blockIndex} block={block} />
              ))}
            </section>
          ))}
        </main>
      </div>
    </div>
  )
}
