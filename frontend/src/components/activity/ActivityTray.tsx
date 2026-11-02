import { useId, useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronUp } from 'lucide-react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useActivity } from '@/hooks/useActivity'
import { cn } from '@/lib/utils'
import type { ActivityItem } from '@/types/activity'
import { activityLabel } from './activity-labels'
import { ActivityCard } from './ActivityCard'
import { useDismissedActivity } from './useDismissedActivity'
import { useRefreshListsOnSettle } from './useRefreshListsOnSettle'

const MAX_VISIBLE_CARDS = 4

/**
 * What the tray says about itself, read from the items' real status: a failure
 * first, then how many are still running, and "Ready" once nothing is. It is
 * the only text a phone shows until the pill is opened.
 */
function trayStatus(items: ActivityItem[], t: TFunction): { label: string; failed: boolean } {
  const failed = items.filter((item) => item.status === 'failed')
  const first = failed[0]
  if (first) {
    const label = activityLabel(first, t)
    return {
      label: failed.length > 1 ? `${label} ${t('activity.more', { count: failed.length - 1 })}` : label,
      failed: true,
    }
  }
  const running = items.filter((item) => item.status === 'in_progress').length
  return { label: running > 0 ? t('activity.count', { count: running }) : t('activity.ready'), failed: false }
}

/** On phones the pill already gives the count; the header stays there only
 *  while it carries the overflow link. */
function TrayHeader({ label, overflow }: { label: string; overflow: number }) {
  const { t } = useTranslation('common')
  return (
    <div
      className={cn(
        'flex items-center justify-between rounded-lg border border-background/20 bg-foreground px-3 py-2 text-sm text-background shadow-lg',
        overflow === 0 && 'max-md:hidden',
      )}
    >
      <span className="font-semibold">{label}</span>
      {overflow > 0 && (
        <Button asChild variant="link" size="sm" className="h-7 px-0 text-background hover:text-background/80">
          <Link to="/library">{t('activity.more', { count: overflow })}</Link>
        </Button>
      )}
    </div>
  )
}

interface TrayPillProps {
  label: string
  failed: boolean
  running: boolean
  expanded: boolean
  cardsId: string
  onToggle: () => void
}

/** Phones only: the tray folded into one line, which opens the cards on tap. */
function TrayPill({ label, failed, running, expanded, cardsId, onToggle }: TrayPillProps) {
  return (
    <button
      type="button"
      aria-expanded={expanded}
      aria-controls={cardsId}
      onClick={onToggle}
      className="pointer-events-auto inline-flex min-h-11 max-w-full items-center gap-2 rounded-full border border-background/20 bg-foreground px-4 py-1.5 text-sm font-semibold text-background shadow-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 md:hidden"
    >
      {running && (
        <span
          aria-hidden="true"
          className="h-1.5 w-1.5 rounded-full border border-current motion-safe:animate-[status-pulse_1.8s_ease-in-out_infinite]"
        />
      )}
      <span
        data-testid="activity-pill-label"
        className={cn('min-w-0 text-left', failed && 'text-destructive-inverse')}
      >
        {label}
      </span>
      <ChevronUp
        aria-hidden="true"
        className={cn('h-4 w-4 shrink-0 transition-transform duration-200 motion-reduce:transition-none', expanded && 'rotate-180')}
      />
    </button>
  )
}

export function ActivityTray() {
  const { t } = useTranslation('common')
  const { data } = useActivity()
  // The Library and session lists cache their own queries; refresh them the
  // moment an item here finishes or fails so no row sits on a stale status.
  useRefreshListsOnSettle(data?.items)
  const { visibleItems, dismiss } = useDismissedActivity(data?.items)
  // Phones only: the tray folds into one pill so it never covers the page.
  const [expanded, setExpanded] = useState(false)
  const cardsId = useId()

  if (visibleItems.length === 0) return null

  const cards = visibleItems.slice(0, MAX_VISIBLE_CARDS)
  const overflow = visibleItems.length - cards.length

  const running = visibleItems.some((item) => item.status === 'in_progress')
  const status = trayStatus(visibleItems, t)

  return (
    // `data-activity-tray` lets the reader's MiniDock keep clear of the tray
    // only while it is actually on screen. On phones the tray rises above that
    // dock when it shows. The aside itself ignores the pointer so the empty
    // part of its box never blocks the page under it. On phones the pill's text
    // is what this live region announces, because the cards are not rendered.
    <aside
      aria-live="polite"
      data-activity-tray=""
      className="pointer-events-none fixed bottom-4 right-4 z-50 flex w-80 max-w-[calc(100vw-2rem)] flex-col items-end gap-2 max-md:[body:has([data-testid=reader-mini-dock])_&]:bottom-[4.25rem]"
    >
      <div
        id={cardsId}
        className={cn('pointer-events-auto w-full space-y-2', !expanded && 'max-md:hidden')}
      >
        {visibleItems.length > 1 && <TrayHeader label={status.label} overflow={overflow} />}
        <div className={cn('space-y-2', visibleItems.length > 1 && 'max-h-[70vh] overflow-hidden')}>
          {cards.map((item) => (
            <ActivityCard key={item.ref_id} item={item} onDismiss={dismiss} />
          ))}
        </div>
      </div>
      <TrayPill
        label={status.label}
        failed={status.failed}
        running={running}
        expanded={expanded}
        cardsId={cardsId}
        onToggle={() => setExpanded((open) => !open)}
      />
    </aside>
  )
}
