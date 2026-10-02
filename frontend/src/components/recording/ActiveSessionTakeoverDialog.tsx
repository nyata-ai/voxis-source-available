import { useTranslation } from 'react-i18next'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { useFormatters } from '@/i18n/useFormatters'
import type { RecordingSession } from '@/types/recording'

interface ActiveSessionTakeoverDialogProps {
  /** The still-active server session, or null when there is nothing to decide. */
  session: RecordingSession | null
  /** Interrupt the session so it becomes recoverable here. */
  onTakeOver: () => void
  /** Leave the session alone and navigate away from the recorder. */
  onLeave: () => void
  isBusy?: boolean
}

/**
 * Shown when the server still reports an active recording session on a fresh
 * mount of the recorder.
 *
 * A fresh mount is not proof the session is stale: it may be a healthy
 * recording running in another tab. Interrupting it silently would keep that
 * tab capturing audio it can no longer save, so the decision belongs to the
 * user. There is no dismiss-and-stay option — staying would only produce a 409
 * on Start.
 */
export function ActiveSessionTakeoverDialog({
  session,
  onTakeOver,
  onLeave,
  isBusy = false,
}: ActiveSessionTakeoverDialogProps) {
  const { t } = useTranslation('recording')
  const formatters = useFormatters()
  if (!session) return null

  const chunkCount = session.chunk_count ?? 0

  return (
    <AlertDialog open>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('activeTakeover.title')}</AlertDialogTitle>
          <AlertDialogDescription>{t('activeTakeover.description')}</AlertDialogDescription>
        </AlertDialogHeader>
        <div className="space-y-1 text-sm">
          <p>
            <span className="text-muted-foreground">{t('activeTakeover.started')}</span>{' '}
            {formatters.date(session.created_at)}
          </p>
          {chunkCount > 0 && (
            <p className="text-muted-foreground">
              {t('activeTakeover.chunksUploaded', { n: chunkCount })}
            </p>
          )}
          <p className="text-destructive">{t('activeTakeover.warning')}</p>
        </div>
        <AlertDialogFooter className="gap-2 sm:gap-2">
          <AlertDialogCancel onClick={onLeave} disabled={isBusy}>
            {t('activeTakeover.leave')}
          </AlertDialogCancel>
          <Button variant="destructive" onClick={onTakeOver} disabled={isBusy}>
            {isBusy ? t('activeTakeover.takingOver') : t('activeTakeover.takeOver')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
