import { useTranslation } from 'react-i18next'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

/** Which irreversible give-up path the user is about to take. */
export type RecordingDataLossAction = 'savePartial' | 'discard'

interface RecordingDataLossDialogProps {
  action: RecordingDataLossAction | null
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
  isBusy?: boolean
}

/**
 * Confirms the two lossy ways out of a stuck upload: keep only what the server
 * already has, or throw the whole recording away. Both destroy audio, so the
 * consequence is spelled out before either runs.
 */
export function RecordingDataLossDialog({
  action,
  onOpenChange,
  onConfirm,
  isBusy = false,
}: RecordingDataLossDialogProps) {
  const { t } = useTranslation(['recording', 'common'])
  if (!action) return null

  const isDiscard = action === 'discard'

  return (
    <AlertDialog open onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {isDiscard ? t('dataLoss.discardTitle') : t('dataLoss.savePartialTitle')}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {isDiscard
              ? t('dataLoss.discardDescription')
              : t('dataLoss.savePartialDescription')}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isBusy}>{t('common:actions.cancel')}</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            disabled={isBusy}
            className={
              isDiscard
                ? 'bg-destructive text-destructive-foreground hover:bg-destructive/90'
                : undefined
            }
          >
            {isDiscard ? t('dataLoss.confirmDiscard') : t('dataLoss.confirmSavePartial')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
