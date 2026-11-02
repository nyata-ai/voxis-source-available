import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ArrowRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useSummariesByTranscription } from '@/hooks/useSummary'
import { SUMMARY_TYPE_ORDER, getSummaryTypeMeta } from '@/components/summary/SummaryTypeMeta'
import { formatDuration } from '@/lib/utils'
import type { SummaryItem, SummaryType } from '@/types/summary'
import type { LibraryItem } from './library'

/** Completed briefing types, in the reader's tab order. */
function readyBriefingTypes(summaries: SummaryItem[] | undefined): SummaryType[] {
  if (!summaries) return []
  const ready = new Set(summaries.filter((s) => s.status === 'completed').map((s) => s.summary_type))
  return SUMMARY_TYPE_ORDER.filter((type) => ready.has(type))
}

interface ContinueCardProps {
  item: LibraryItem
}

/**
 * The Desk's signature surface for returning users: an ink card that opens
 * the latest finished session. It names the session, its length and speaker
 * count, and which briefings are ready — and says nothing about briefings
 * when none exist (editions without summaries, or none generated yet).
 */
export function ContinueCard({ item }: ContinueCardProps) {
  const { t } = useTranslation(['auth', 'common', 'summary'])
  // `signature` is the transcription's id on a completed transcription row. A
  // privilege session has no briefings and the server answers 404, so don't ask.
  const { data: summaries } = useSummariesByTranscription(item.privilege ? '' : item.signature)
  const briefings = readyBriefingTypes(summaries)
  const title = item.title.trim() || t('common:session.untitled')

  const meta = [
    item.durationSeconds && item.durationSeconds > 0 ? formatDuration(item.durationSeconds) : null,
    item.speakerCount && item.speakerCount > 0
      ? t('common:session.speaker', { count: item.speakerCount })
      : null,
  ].filter(Boolean)

  return (
    <section
      aria-labelledby="continue-heading"
      // Ink in both themes (the reader's audio-card surface). In dark mode the
      // ink sits close to the night canvas, so a hairline keeps its edge.
      className="rounded-lg border border-transparent bg-ink p-6 text-white dark:border-border"
    >
      <h2 id="continue-heading" className="kicker text-white/75">
        {t('auth:dashboard.continue.kicker')}
      </h2>
      <div className="mt-3 flex flex-wrap items-end justify-between gap-x-6 gap-y-4">
        <div className="min-w-0 flex-1">
          <p className="text-page font-normal break-words">{title}</p>
          {meta.length > 0 && (
            <p className="mt-2 text-body tabular-nums text-white/80">{meta.join(' · ')}</p>
          )}
          {briefings.length > 0 && (
            <div className="mt-3 flex flex-wrap items-center gap-1.5">
              <span className="text-body text-white/80">{t('auth:dashboard.continue.briefingsReady')}</span>
              <ul className="flex flex-wrap gap-1.5">
                {briefings.map((type) => (
                  <li
                    key={type}
                    className="rounded-full border border-white/25 px-2.5 py-0.5 text-caption font-semibold text-white"
                  >
                    {t(`summary:${getSummaryTypeMeta(type).shortLabelKey}`)}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
        <Button
          asChild
          className="shrink-0 bg-white text-ink hover:bg-white/90 focus-visible:ring-white focus-visible:ring-offset-ink"
        >
          <Link to={item.to} aria-label={t('auth:dashboard.continue.openLabel', { title })}>
            {t('auth:dashboard.continue.open')}
            <ArrowRight aria-hidden="true" />
          </Link>
        </Button>
      </div>
    </section>
  )
}
