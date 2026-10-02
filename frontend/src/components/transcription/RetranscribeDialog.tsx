import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
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
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { apiClient } from '@/lib/api-client'
import { toast } from '@/lib/toast'
import { transcriptionKeys } from '@/hooks/useTranscription'
import { LanguageSelector } from './LanguageSelector'
import { ExpectedSpeakersSelector } from './ExpectedSpeakersSelector'
import type { TranscriptionItem } from '@/types/transcription'

interface RetranscribeDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  transcription: TranscriptionItem
}

export function RetranscribeDialog({
  open,
  onOpenChange,
  transcription,
}: RetranscribeDialogProps) {
  const { t } = useTranslation(['transcription', 'common'])
  const [languages, setLanguages] = useState<string[]>(transcription.languages)
  const [diarization, setDiarization] = useState(transcription.diarization)
  // Seeded from the original submission: a retranscription that quietly drops
  // the declared count would diarize differently than the run being replaced.
  const [expectedSpeakers, setExpectedSpeakers] = useState(transcription.expected_speakers ?? 0)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  useEffect(() => {
    if (open) {
      setLanguages(transcription.languages)
      setDiarization(transcription.diarization)
      setExpectedSpeakers(transcription.expected_speakers ?? 0)
    }
  }, [open, transcription.languages, transcription.diarization, transcription.expected_speakers])

  const handleSubmit = async () => {
    setIsSubmitting(true)
    try {
      await apiClient.delete(`/transcriptions/${transcription.id}`)
    } catch {
      setIsSubmitting(false)
      toast.error(t('retranscribe.deleteFailed'))
      return
    }

    try {
      const newTrans = await apiClient.post<TranscriptionItem>('/transcriptions', {
        media_id: transcription.media_id,
        languages,
        diarization,
        // Only sent when a count is in play; the server ignores it without
        // diarization anyway.
        ...(diarization && expectedSpeakers > 0 ? { expected_speakers: expectedSpeakers } : {}),
      })
      queryClient.invalidateQueries({ queryKey: transcriptionKeys.all })
      onOpenChange(false)
      toast.success(t('retranscribe.started'))
      navigate(`/transcriptions/${newTrans.id}`)
    } catch {
      queryClient.invalidateQueries({ queryKey: transcriptionKeys.all })
      onOpenChange(false)
      toast.error(t('retranscribe.newFailed'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent className="max-h-[85vh] overflow-y-auto">
        <AlertDialogHeader>
          <AlertDialogTitle>{t('retranscribe.title')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t('retranscribe.descriptionPrefix')}{' '}
            <strong>{transcription.media_filename}</strong>{' '}
            {t('retranscribe.descriptionSuffix')}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="space-y-4 py-4">
          <LanguageSelector languages={languages} onChange={setLanguages} />
          <div className="flex items-center gap-2">
            <Checkbox
              id="retranscribe-diarization"
              checked={diarization}
              onCheckedChange={(checked) => setDiarization(checked === true)}
            />
            <Label htmlFor="retranscribe-diarization">{t('retranscribe.diarization')}</Label>
          </div>
          {diarization && (
            <ExpectedSpeakersSelector
              value={expectedSpeakers}
              onChange={setExpectedSpeakers}
              idPrefix="retranscribe"
              disabled={isSubmitting}
            />
          )}
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isSubmitting}>{t('common:actions.cancel')}</AlertDialogCancel>
          <Button
            onClick={handleSubmit}
            disabled={isSubmitting || languages.length === 0}
          >
            {isSubmitting ? t('retranscribe.starting') : t('retranscribe.start')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
