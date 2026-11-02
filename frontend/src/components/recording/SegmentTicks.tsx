import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

/** The most ticks drawn at once (an 8-hour recording has 960 segments). The
 *  newest ones are shown; the caption keeps the full count. */
const MAX_TICKS = 80

interface SegmentTicksProps {
  /** Segments captured so far (one per 30-second slice). */
  total: number
  /** Segments the server has accepted. */
  saved: number
  /** Unsent segments: past the backlog limit the caption turns to a warning. */
  unsent: number
  backlogLimit: number
}

/**
 * One small tick per 30-second segment under the tape: hollow while it waits
 * on this device, solid ink once the server has it. The server encrypts each
 * segment as it arrives, so "saved" here means stored encrypted. Warning colour
 * appears only once the backlog passes the limit that pauses the recording.
 */
export function SegmentTicks({ total, saved, unsent, backlogLimit }: SegmentTicksProps) {
  const { t } = useTranslation('recording')
  const shown = Math.min(total, MAX_TICKS)
  const firstShown = total - shown
  const overLimit = unsent > backlogLimit

  return (
    <div className="flex w-full max-w-[640px] flex-col items-center gap-2">
      <div
        aria-hidden="true"
        data-testid="segment-ticks"
        className="flex h-2 w-full items-end justify-end gap-1 border-b border-border"
      >
        {Array.from({ length: shown }, (_, i) => {
          const index = firstShown + i
          return (
            <span
              key={index}
              data-saved={index < saved ? 'true' : 'false'}
              className={cn(
                'h-2 w-0.5 shrink-0 transition-colors duration-240 motion-reduce:transition-none',
                // At 2px wide a ring would read as solid, so a waiting
                // segment is a faint tick instead.
                index < saved ? 'bg-foreground' : 'bg-muted-foreground/40',
              )}
            />
          )
        })}
      </div>
      <p className={cn('text-caption', overLimit ? 'text-warning' : 'text-muted-foreground')}>
        {overLimit
          ? t('upload.saving', { n: unsent })
          : total > 0
            ? `${t('tape.caption')} · ${t('tape.count', { saved, total })}`
            : t('tape.caption')}
      </p>
    </div>
  )
}
