import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/lib/toast'
import type { ActivityItem } from '@/types/activity'
import { activityIntake, activityLabel, activitySteps, elapsedLabel } from './activity-labels'
import { IntakeDisc } from './IntakeDisc'
import { StepTrack } from './StepTrack'

interface ActivityCardProps {
  item: ActivityItem
  onDismiss: (refID: string) => void
}

const TERMINAL_DISMISS_MS = 10_000

function titleForItem(item: ActivityItem, fallback: string): string {
  return item.title?.trim() || fallback
}

/** How long the job has been running, ticking once a second while it runs. */
function useElapsed(item: ActivityItem): string {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (item.status !== 'in_progress') return
    setNow(Date.now())
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [item.status])

  return useMemo(() => elapsedLabel(item.started_at, now), [item.started_at, now])
}

/** The toast when a job this card watched finishes, and the card's own exit. */
function useCompletion(item: ActivityItem, onDismiss: ActivityCardProps['onDismiss']): void {
  const { t } = useTranslation('common')
  const wasInProgress = useRef(false)
  const didToastReady = useRef(false)

  useEffect(() => {
    if (item.status === 'in_progress') {
      wasInProgress.current = true
      return
    }
    if (item.status !== 'completed' || !wasInProgress.current || didToastReady.current) return
    didToastReady.current = true
    toast.success(
      item.kind === 'recording'
        ? t('activity.toast.recordingReady')
        : t('activity.toast.transcriptionReady'),
    )
  }, [item.kind, item.ref_id, item.status, t])

  // Only success dismisses itself. A failure stays until the reader dismisses
  // it (remembered for the session: see useDismissedActivity) or the feed lets it
  // go: on a phone the card is folded behind the pill, and its reason would
  // otherwise vanish before anyone opened it.
  useEffect(() => {
    if (item.status !== 'completed') return
    const timer = window.setTimeout(() => onDismiss(item.ref_id), TERMINAL_DISMISS_MS)
    return () => window.clearTimeout(timer)
  }, [item.ref_id, item.status, onDismiss])
}

function CardActions({ item, onDismiss }: ActivityCardProps) {
  const { t } = useTranslation('common')
  return (
    <div className="mt-3 flex items-center gap-2">
      {/* Failed items get View too: the session page carries the full
          reason and the retry controls this card deliberately lacks. */}
      {item.status !== 'in_progress' && item.link && (
        <Button
          asChild
          variant="secondary"
          size="sm"
          className="h-11 bg-background/15 px-2.5 text-background md:h-8 hover:bg-background/25 hover:text-background"
        >
          <Link to={item.link}>{t('activity.view')}</Link>
        </Button>
      )}
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="h-11 px-2.5 text-background hover:bg-background/15 md:h-8 hover:text-background"
        onClick={() => onDismiss(item.ref_id)}
      >
        <X className="h-3.5 w-3.5" />
        {t('activity.dismiss')}
      </Button>
    </div>
  )
}

export function ActivityCard({ item, onDismiss }: ActivityCardProps) {
  const { t } = useTranslation('common')
  const title = titleForItem(item, item.kind === 'recording' ? t('activity.recordingTitle') : t('activity.title'))
  const elapsed = useElapsed(item)
  useCompletion(item, onDismiss)

  return (
    <article
      data-testid="activity-card"
      className="rounded-lg border border-background/20 bg-foreground p-3 text-background shadow-lg"
    >
      <div className="flex items-start gap-3">
        <IntakeDisc intake={activityIntake(item)} className="mt-px" />

        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{title}</p>
          <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-background/70">
            <StepTrack
              steps={activitySteps(item)}
              label={activityLabel(item, t)}
              surface="ink"
              labelClassName={item.status === 'failed' ? 'font-semibold text-destructive-inverse' : undefined}
            />
            {item.status === 'in_progress' && elapsed && (
              <>
                <span aria-hidden="true" className="opacity-50">·</span>
                <span>{elapsed}</span>
              </>
            )}
          </div>

          {/* The stored reason is the only actionable thing on a failed card —
              clamped to two lines here, in full on hover and on the session
              page behind View. */}
          {item.status === 'failed' && item.error_message && (
            <p
              data-testid="activity-error-message"
              title={item.error_message}
              className="mt-1 line-clamp-2 text-xs text-background/70"
            >
              {item.error_message}
            </p>
          )}

          <CardActions item={item} onDismiss={onDismiss} />
        </div>
      </div>
    </article>
  )
}
