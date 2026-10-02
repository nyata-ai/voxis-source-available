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

interface MeetingCaptureConsentDialogProps {
  open: boolean
  onConfirm: () => void
  onCancel: () => void
}

/** A just-in-time reminder before the browser asks which meeting audio to share. */
export function MeetingCaptureConsentDialog({
  open,
  onConfirm,
  onCancel,
}: MeetingCaptureConsentDialogProps) {
  const { t } = useTranslation('recording')

  return (
    <AlertDialog open={open} onOpenChange={(nextOpen) => !nextOpen && onCancel()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('meetingConsent.title')}</AlertDialogTitle>
          <AlertDialogDescription>{t('meetingConsent.description')}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t('meetingConsent.cancel')}</AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm}>{t('meetingConsent.continue')}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
