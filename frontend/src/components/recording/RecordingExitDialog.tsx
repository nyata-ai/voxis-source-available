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

interface RecordingExitDialogProps {
  open: boolean
  /** Keep recording and stay on the page. */
  onKeepRecording: () => void
  /** Run the normal stop flow: drain the outbox, then complete the session. */
  onStopAndSave: () => void
  /** Throw the recording away (release + abandon on the server). */
  onDiscard: () => void
  isBusy?: boolean
}

/**
 * Shown when the user tries to navigate away while a recording is live.
 *
 * Leaving without one of these two outcomes strands the session on the server
 * in `recording` and leaves the captured audio unreachable, so the dialog has
 * no "leave anyway" escape hatch.
 */
export function RecordingExitDialog({
  open,
  onKeepRecording,
  onStopAndSave,
  onDiscard,
  isBusy = false,
}: RecordingExitDialogProps) {
  const { t } = useTranslation('recording')

  return (
    <AlertDialog open={open}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('exit.title')}</AlertDialogTitle>
          <AlertDialogDescription>{t('exit.description')}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter className="gap-2 sm:gap-2">
          <AlertDialogCancel onClick={onKeepRecording} disabled={isBusy}>
            {t('exit.keep')}
          </AlertDialogCancel>
          <Button variant="destructive" onClick={onDiscard} disabled={isBusy}>
            {t('exit.discard')}
          </Button>
          <Button onClick={onStopAndSave} disabled={isBusy}>
            {t('exit.stopAndSave')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
