import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import type { RecordingSession } from '@/types/recording'
import { useFormatters } from '@/i18n/useFormatters'

interface RecoverRecordingDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  sessions: RecordingSession[]
  onRecover: (sessionId: string) => void
  onDiscard: (sessionId: string) => void
  isRecovering: boolean
  isDiscarding: boolean
}

/**
 * Dialog for handling recovery of interrupted recording sessions.
 *
 * Single session: shows summary with Recover / Discard buttons.
 * Multiple sessions: list view with per-session actions.
 */
export function RecoverRecordingDialog({
  open,
  onOpenChange,
  sessions,
  onRecover,
  onDiscard,
  isRecovering,
  isDiscarding,
}: RecoverRecordingDialogProps) {
  const { t } = useTranslation('recording')
  const isBusy = isRecovering || isDiscarding

  if (sessions.length === 0) return null

  const isSingle = sessions.length === 1

  return (
    <Dialog open={open} onOpenChange={isBusy ? undefined : onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {isSingle ? t('recover.titleSingle') : t('recover.titleMulti')}
          </DialogTitle>
          <DialogDescription>
            {isSingle
              ? t('recover.descriptionSingle')
              : t('recover.descriptionMulti', { n: sessions.length })}
          </DialogDescription>
        </DialogHeader>

        {isSingle ? (
          <SingleSessionView
            session={sessions[0]}
            onRecover={onRecover}
            onDiscard={onDiscard}
            isRecovering={isRecovering}
            isDiscarding={isDiscarding}
          />
        ) : (
          <MultiSessionView
            sessions={sessions}
            onRecover={onRecover}
            onDiscard={onDiscard}
            isBusy={isBusy}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function SingleSessionView({
  session,
  onRecover,
  onDiscard,
  isRecovering,
  isDiscarding,
}: {
  session: RecordingSession
  onRecover: (id: string) => void
  onDiscard: (id: string) => void
  isRecovering: boolean
  isDiscarding: boolean
}) {
  const { t } = useTranslation('recording')
  const formatters = useFormatters()
  const duration = estimateDuration(session)
  const isBusy = isRecovering || isDiscarding

  return (
    <>
      <div className="space-y-1 text-sm">
        <p>
          <span className="text-muted-foreground">{t('recover.date')}</span>{' '}
          {formatters.date(session.created_at)}
        </p>
        {duration && (
          <p>
            <span className="text-muted-foreground">{t('recover.estimatedDuration')}</span>{' '}
            {duration}
          </p>
        )}
        {session.microphone_label && (
          <p>
            <span className="text-muted-foreground">{t('recover.microphone')}</span>{' '}
            {session.microphone_label}
          </p>
        )}
      </div>
      <DialogFooter className="gap-2 sm:gap-0">
        <Button
          variant="outline"
          onClick={() => onDiscard(session.id)}
          disabled={isBusy}
        >
          {isDiscarding ? t('recover.discarding') : t('recover.discard')}
        </Button>
        <Button
          onClick={() => onRecover(session.id)}
          disabled={isBusy}
        >
          {isRecovering ? t('recover.recovering') : t('recover.recover')}
        </Button>
      </DialogFooter>
    </>
  )
}

function MultiSessionView({
  sessions,
  onRecover,
  onDiscard,
  isBusy,
}: {
  sessions: RecordingSession[]
  onRecover: (id: string) => void
  onDiscard: (id: string) => void
  isBusy: boolean
}) {
  const { t } = useTranslation('recording')
  const formatters = useFormatters()
  return (
    <div className="space-y-3 max-h-64 overflow-y-auto">
      {sessions.map((session) => {
        const duration = estimateDuration(session)

        return (
          <div
            key={session.id}
            className="flex items-center justify-between rounded-lg border p-3"
          >
            <div className="space-y-0.5 text-sm">
              <p className="font-medium">{formatters.date(session.created_at)}</p>
              {duration && (
                <p className="text-muted-foreground">{duration}</p>
              )}
            </div>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => onDiscard(session.id)}
                disabled={isBusy}
              >
                {t('recover.discard')}
              </Button>
              <Button
                size="sm"
                onClick={() => onRecover(session.id)}
                disabled={isBusy}
              >
                {t('recover.recover')}
              </Button>
            </div>
          </div>
        )
      })}
    </div>
  )
}

/**
 * Estimate duration from total_duration or last_chunk_at.
 * During recording, total_duration is 0 until stitching finishes.
 */
function estimateDuration(session: RecordingSession): string | null {
  if (session.total_duration > 0) {
    return formatSeconds(session.total_duration)
  }

  // Estimate from elapsed time
  if (session.last_chunk_at) {
    const start = new Date(session.created_at).getTime()
    const last = new Date(session.last_chunk_at).getTime()
    const seconds = Math.floor((last - start) / 1000)
    if (seconds > 0) {
      return `~${formatSeconds(seconds)}`
    }
  }

  return null
}

function formatSeconds(totalSeconds: number): string {
  const h = Math.floor(totalSeconds / 3600)
  const m = Math.floor((totalSeconds % 3600) / 60)
  const s = Math.floor(totalSeconds % 60)

  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}
