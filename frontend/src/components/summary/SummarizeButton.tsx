import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Sparkles } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Label } from '@/components/ui/label'
import { useCreateSummary } from '@/hooks/useSummary'
import { getApiErrorMessage } from '@/lib/api-errors'
import { toast } from '@/lib/toast'
import { SUMMARY_TYPE_ORDER, getSummaryTypeMeta } from './SummaryTypeMeta'
import type { SummaryType } from '@/types/summary'

// Derived, not restated: `SummaryTypeMeta` is the one place a summary type's
// order, icon and labels are declared, so a fifth type is added there alone.
const SUMMARY_TYPES: { value: SummaryType; labelKey: string }[] = SUMMARY_TYPE_ORDER.map(
  (value) => ({ value, labelKey: getSummaryTypeMeta(value).labelKey })
)

interface SummarizeButtonProps {
  transcriptionId: string
  transcriptionStatus: string
  existingSummaryTypes?: string[]
  onSuccess?: () => void
}

export function SummarizeButton({
  transcriptionId,
  transcriptionStatus,
  existingSummaryTypes = [],
  onSuccess,
}: SummarizeButtonProps) {
  const { t } = useTranslation(['summary', 'common', 'errors'])
  const availableTypes = useMemo(
    () => SUMMARY_TYPES.filter((type) => !existingSummaryTypes.includes(type.value)),
    [existingSummaryTypes]
  )

  const [open, setOpen] = useState(false)
  const [summaryType, setSummaryType] = useState<SummaryType>(
    availableTypes[0]?.value ?? 'general'
  )
  const createMutation = useCreateSummary()

  useEffect(() => {
    if (availableTypes.some((type) => type.value === summaryType)) return
    setSummaryType(availableTypes[0]?.value ?? 'general')
  }, [availableTypes, summaryType])

  if (transcriptionStatus !== 'completed') return null
  if (availableTypes.length === 0) return null

  const handleSubmit = () => {
    if (existingSummaryTypes.includes(summaryType)) {
      const nextType = availableTypes[0]?.value
      if (nextType) setSummaryType(nextType)
      toast.error(t('summarize.alreadyExists'))
      return
    }

    createMutation.mutate(
      { transcription_id: transcriptionId, summary_type: summaryType },
      {
        onSuccess: () => {
          setOpen(false)
          toast.success(t('summarize.started'))
          onSuccess?.()
        },
        onError: (error: Error) => {
          toast.error(getApiErrorMessage(error, t, t('summarize.failed')))
        },
      }
    )
  }

  return (
    <AlertDialog open={open} onOpenChange={setOpen}>
      <AlertDialogTrigger asChild>
        <Button>
          <Sparkles className="mr-2 h-4 w-4" /> {t('summarize.button')}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('summarize.title')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t('summarize.description')}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="summary-type">{t('summarize.typeLabel')}</Label>
            <Select value={summaryType} onValueChange={(v) => setSummaryType(v as SummaryType)}>
              <SelectTrigger id="summary-type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SUMMARY_TYPES.map((type) => (
                  <SelectItem
                    key={type.value}
                    value={type.value}
                    disabled={existingSummaryTypes.includes(type.value)}
                  >
                    {t(type.labelKey)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={createMutation.isPending}>{t('common:actions.cancel')}</AlertDialogCancel>
          <AlertDialogAction onClick={handleSubmit} disabled={createMutation.isPending}>
            {createMutation.isPending ? t('summarize.generating') : t('summarize.start')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
