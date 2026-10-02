import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { AlertCircle, CheckCircle2, Loader2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { toast } from '@/lib/toast'
import type { ActivityItem } from '@/types/activity'
import { activityStageKey, activityStatusTone, elapsedLabel } from './activity-labels'

interface ActivityCardProps {
  item: ActivityItem
  onDismiss: (refID: string) => void
}

const TERMINAL_DISMISS_MS = 10_000

function titleForItem(item: ActivityItem, fallback: string): string {
  return item.title?.trim() || fallback
}

export function ActivityCard({ item, onDismiss }: ActivityCardProps) {
  const { t } = useTranslation('common')
  const wasInProgress = useRef(false)
  const didToastReady = useRef(false)
  const [now, setNow] = useState(() => Date.now())
  const tone = activityStatusTone(item.status)
  const stageText = t(activityStageKey(item.stage), {
    terminal: item.detail?.terminal ?? 0,
    total: item.detail?.total ?? 0,
  })
  const title = titleForItem(item, item.kind === 'recording' ? t('activity.recordingTitle') : t('activity.title'))
  const elapsed = useMemo(() => elapsedLabel(item.started_at, now), [item.started_at, now])

  useEffect(() => {
    if (item.status !== 'in_progress') return
    setNow(Date.now())
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [item.status])

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

  useEffect(() => {
    if (item.status !== 'completed' && item.status !== 'failed') return
    const timer = window.setTimeout(() => onDismiss(item.ref_id), TERMINAL_DISMISS_MS)
    return () => window.clearTimeout(timer)
  }, [item.ref_id, item.status, onDismiss])

  return (
    <article
      data-testid="activity-card"
      className="rounded-lg border border-background/20 bg-foreground p-3 text-background shadow-lg"
    >
      <div className="flex items-start gap-3">
        <span
          className={cn(
            'mt-0.5 inline-flex h-5 w-5 items-center justify-center',
            tone === 'success' && 'text-background/70',
            tone === 'error' && 'text-red-300 [.dark_&]:text-red-800',
            tone === 'spinner' && 'text-background',
          )}
        >
          {tone === 'spinner' && (
            <Loader2
              aria-hidden="true"
              className="h-4 w-4 motion-safe:animate-spin motion-reduce:animate-none"
            />
          )}
          {tone === 'success' && <CheckCircle2 aria-hidden="true" className="h-4 w-4" />}
          {tone === 'error' && <AlertCircle aria-hidden="true" className="h-4 w-4" />}
        </span>

        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{title}</p>
          <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-background/70">
            <span
              className={cn(
                item.status === 'completed' && 'font-medium text-background/70',
                item.status === 'failed' && 'font-medium text-red-300 [.dark_&]:text-red-800',
              )}
            >
              {item.status === 'completed'
                ? t('activity.ready')
                : item.status === 'failed'
                  ? t(`activity.failed.${item.kind}`)
                  : stageText}
            </span>
            {item.status === 'in_progress' && elapsed && (
              <>
                <span aria-hidden="true" className="opacity-50">·</span>
                <span>{elapsed}</span>
              </>
            )}
          </div>

          {item.status === 'failed' && item.error_message && (
            <p
              data-testid="activity-error-message"
              title={item.error_message}
              className="mt-1 line-clamp-2 text-xs text-background/70"
            >
              {item.error_message}
            </p>
          )}

          <div className="mt-3 flex items-center gap-2">
            {item.status !== 'in_progress' && item.link && (
              <Button
                asChild
                variant="secondary"
                size="sm"
                className="h-8 bg-background/15 px-2.5 text-background hover:bg-background/25 hover:text-background"
              >
                <Link to={item.link}>{t('activity.view')}</Link>
              </Button>
            )}
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-8 px-2.5 text-background hover:bg-background/15 hover:text-background"
              onClick={() => onDismiss(item.ref_id)}
            >
              <X className="h-3.5 w-3.5" />
              {t('activity.dismiss')}
            </Button>
          </div>
        </div>
      </div>
    </article>
  )
}
