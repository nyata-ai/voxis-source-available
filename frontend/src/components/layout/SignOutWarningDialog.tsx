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

interface SignOutWarningDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}

/**
 * Shown before sign-out when this browser still holds recording audio that
 * never reached the server. Signing out deletes it from the browser (so the
 * next person on a shared computer cannot read it), and it cannot come back.
 */
export function SignOutWarningDialog({ open, onOpenChange, onConfirm }: SignOutWarningDialogProps) {
  const { t } = useTranslation('auth')
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('signOutWarning.title')}</AlertDialogTitle>
          <AlertDialogDescription>{t('signOutWarning.description')}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t('signOutWarning.stay')}</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {t('signOutWarning.confirm')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
