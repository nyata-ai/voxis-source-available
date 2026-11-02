import type { MouseEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { scrollBehavior } from '@/hooks/useTranscriptAutoScroll'

export interface ReaderSection {
  /** DOM id of the section the link scrolls to. */
  id: string
  /** `transcription:reader.sections.*` key. */
  labelKey: string
}

/**
 * "Transcript · Briefings · Export": in-page links in the reader's sticky bar,
 * so the briefings below the workspace are one click away. A link scrolls
 * instead of writing a `#hash`, which would stay in the address bar and pull
 * the page back on refresh. Sections the session does not have are not passed
 * in; with fewer than two there is nothing to jump between and nothing draws.
 */
export function ReaderSectionLinks({ sections }: { sections: ReaderSection[] }) {
  const { t } = useTranslation(['transcription'])
  if (sections.length < 2) return null

  const jump = (event: MouseEvent<HTMLAnchorElement>, id: string) => {
    const target = document.getElementById(id)
    if (!target) return
    event.preventDefault()
    // `?.()`: jsdom leaves scrollIntoView undefined on plain elements.
    target.scrollIntoView?.({ block: 'start', behavior: scrollBehavior() })
  }

  return (
    <nav
      aria-label={t('transcription:reader.sections.label')}
      data-testid="reader-section-links"
      // Scrolls sideways inside itself if a long translation will not fit a
      // phone, rather than pushing the page wider than the screen.
      className="flex min-w-0 items-center overflow-x-auto"
    >
      <span aria-hidden="true" className="mx-2 h-4 w-px shrink-0 bg-border" />
      <ul className="flex items-center">
        {sections.map((section, index) => (
          <li key={section.id} className="flex shrink-0 items-center">
            {index > 0 && (
              <span aria-hidden="true" className="text-ink-faint">
                ·
              </span>
            )}
            <a
              href={`#${section.id}`}
              onClick={(event) => jump(event, section.id)}
              className="inline-flex min-h-11 items-center rounded-md px-2 text-body text-ink-muted transition-colors hover:text-coral-mark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:text-muted-foreground lg:min-h-9"
            >
              {t(section.labelKey)}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  )
}
