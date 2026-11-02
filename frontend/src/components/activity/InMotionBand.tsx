import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { useActivity } from '@/hooks/useActivity'
import { cn } from '@/lib/utils'
import type { ActivityItem } from '@/types/activity'
import { activityIntake, activityLabel, activitySteps, elapsedLabel } from './activity-labels'
import { IntakeDisc } from './IntakeDisc'
import { StepTrack } from './StepTrack'
import { useRefreshListsOnSettle } from './useRefreshListsOnSettle'

/** Same fallback the row itself displays — kept as one function so the live
 * announcement below never drifts from what's on screen. */
function titleForItem(item: ActivityItem, t: TFunction): string {
  return item.title?.trim() || (item.kind === 'recording' ? t('activity.recordingTitle') : t('activity.title'))
}

/**
 * "In flight" is everything that has not finished — still running, or stopped
 * on a failure. Completed work is deliberately excluded: it already appears in
 * the Recent list directly below the band on the Desk, and showing it twice
 * would double-count the same session.
 */
function isInFlight(item: ActivityItem): boolean {
  return item.status !== 'completed'
}

/**
 * The Desk's inline replacement for the floating ActivityTray: a flat bordered
 * list of everything still moving, in the tray's own stage vocabulary. Renders
 * nothing at all when the desk is quiet — an empty band would be a permanent
 * fixture saying nothing.
 */
export function InMotionBand() {
  const { t } = useTranslation('common')
  const { data } = useActivity()
  const items = useMemo(() => (data?.items ?? []).filter(isInFlight), [data?.items])
  // The Recent list beside this band caches its own query; refresh it (and the
  // Library's) the moment something here finishes or fails.
  useRefreshListsOnSettle(items)
  const [now, setNow] = useState(() => Date.now())
  const [announcement, setAnnouncement] = useState('')
  const prevItemsRef = useRef<Map<string, ActivityItem> | null>(null)

  const hasRunning = items.some((item) => item.status === 'in_progress')

  // One shared 1s tick for the whole band, and only while something is
  // actually running — a failed-only band has no clock to advance.
  useEffect(() => {
    if (!hasRunning) return
    setNow(Date.now())
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [hasRunning])

  // Announce status *transitions* only, never the clock above — that would
  // read the elapsed time aloud every second. Diffs the previous render's
  // items against this one by ref_id: a changed status, a newly appeared
  // item, or one that dropped out of the in-flight list entirely (almost
  // always because it just finished). The first render just seeds the ref
  // so mount never announces anything.
  useEffect(() => {
    const prevItems = prevItemsRef.current
    const nextItems = new Map(items.map((item) => [item.ref_id, item] as const))

    if (prevItems === null) {
      prevItemsRef.current = nextItems
      return
    }

    let message = ''
    for (const item of items) {
      const prev = prevItems.get(item.ref_id)
      if (!prev || prev.status !== item.status) {
        message = `${titleForItem(item, t)}: ${activityLabel(item, t)}`
      }
    }
    for (const [refID, prevItem] of prevItems) {
      if (!nextItems.has(refID)) {
        message = `${titleForItem(prevItem, t)}: ${t('activity.ready')}`
      }
    }

    prevItemsRef.current = nextItems
    if (message) setAnnouncement(message)
  }, [items, t])

  return (
    <>
      {/* Off-screen counterpart to the visible list below, which is deliberately
          not itself an aria-live region (its elapsed clock reticks every second,
          which a live region would read aloud every second). This announces only
          the transitions computed above. */}
      <span aria-live="polite" role="status" className="sr-only">
        {announcement}
      </span>
      {items.length > 0 && (
        <section aria-labelledby="in-motion-heading">
          <h2
            id="in-motion-heading"
            className="kicker mb-3"
          >
            {t('activity.inMotion.title')}
          </h2>
          <ul className="overflow-hidden rounded-lg border border-border bg-card">
            {items.map((item) => (
              <InMotionRow key={item.ref_id} item={item} now={now} />
            ))}
          </ul>
        </section>
      )}
    </>
  )
}

interface InMotionRowProps {
  item: ActivityItem
  now: number
}

function InMotionRow({ item, now }: InMotionRowProps) {
  const { t } = useTranslation('common')
  const failed = item.status === 'failed'
  const title = titleForItem(item, t)
  const label = activityLabel(item, t)
  // A failed item stopped moving, so its elapsed clock would be meaningless.
  const elapsed = failed ? '' : elapsedLabel(item.started_at, now)

  const body = (
    <>
      <IntakeDisc intake={activityIntake(item)} />

      <span className="min-w-0 truncate text-sm text-foreground">{title}</span>

      <span className="flex shrink-0 items-center gap-2 text-caption">
        <StepTrack
          steps={activitySteps(item)}
          label={label}
          labelClassName={failed ? 'font-semibold text-destructive' : 'text-muted-foreground'}
        />
        {elapsed && (
          <>
            <span aria-hidden="true" className="text-muted-foreground">
              ·
            </span>
            <span className="tabular-nums text-muted-foreground">{elapsed}</span>
          </>
        )}
      </span>
    </>
  )

  const rowClass = 'grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2'

  return (
    <li className="border-b border-border/60 last:border-b-0">
      {item.link ? (
        <Link to={item.link} className={cn(rowClass, 'transition-colors hover:bg-accent/30')}>
          {body}
        </Link>
      ) : (
        <div className={rowClass}>{body}</div>
      )}
    </li>
  )
}
