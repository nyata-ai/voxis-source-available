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

interface DeleteConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  filename: string
  onConfirm: () => void
  isDeleting?: boolean
}

export function DeleteConfirmDialog({
  open,
  onOpenChange,
  filename,
  onConfirm,
  isDeleting,
}: DeleteConfirmDialogProps) {
  const { t } = useTranslation(['media', 'common'])
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('delete.title')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t('delete.descriptionBefore')}<strong>{filename}</strong>{t('delete.descriptionAfter')}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isDeleting}>{t('common:actions.cancel')}</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            disabled={isDeleting}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {isDeleting ? t('delete.deleting') : t('common:actions.delete')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
