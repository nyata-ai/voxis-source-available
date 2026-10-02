import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { FileText } from 'lucide-react'
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
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { useCreateTranscription } from '@/hooks/useTranscription'
import { useFeatures } from '@/hooks/useFeatures'
import { getApiErrorMessage } from '@/lib/api-errors'
import { toast } from '@/lib/toast'
import { LanguageSelector } from './LanguageSelector'
import { ExpectedSpeakersSelector } from './ExpectedSpeakersSelector'

interface TranscribeButtonProps {
  mediaId: string
  mediaStatus: string
  onSuccess?: () => void
}

export function TranscribeButton({ mediaId, mediaStatus, onSuccess }: TranscribeButtonProps) {
  const { t } = useTranslation(['transcription', 'common', 'errors'])
  const [open, setOpen] = useState(false)
  const [languages, setLanguages] = useState<string[]>(['auto'])
  const [diarization, setDiarization] = useState(true)
  const [enhanceAudio, setEnhanceAudio] = useState(false)
  const [expectedSpeakers, setExpectedSpeakers] = useState(0)
  const navigate = useNavigate()
  const createMutation = useCreateTranscription()
  const { data: features } = useFeatures()

  if (mediaStatus !== 'ready') return null

  // AlertDialogAction renders as a Radix Dialog.Close under the hood, so a
  // plain onClick would close the dialog synchronously — before the async
  // mutation's onError ever runs, which would make the credit-error banner
  // unrenderable. preventDefault keeps the dialog open through the request;
  // onSuccess closes it explicitly once the job is confirmed started.
  const handleSubmit = (event: React.MouseEvent) => {
    event.preventDefault()
    createMutation.mutate(
      {
        media_id: mediaId,
        languages,
        diarization,
        enhance_audio: enhanceAudio || undefined,
        // Only sent when the user picked a number; the field is meaningless
        // without diarization and the server ignores it there anyway.
        ...(diarization && expectedSpeakers > 0 ? { expected_speakers: expectedSpeakers } : {}),
      },
      {
        onSuccess: (data) => {
          setOpen(false)
          toast.success(t('transcribe.started'))
          onSuccess?.()
          navigate(`/transcriptions/${data.id}`)
        },
        onError: (error: Error) => {
          toast.error(getApiErrorMessage(error, t, t('transcribe.failed')))
        },
      }
    )
  }

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) {
          setEnhanceAudio(false)
          setExpectedSpeakers(0)
        }
      }}
    >
      <AlertDialogTrigger asChild>
        <Button>
          <FileText className="mr-2 h-4 w-4" /> {t('transcribe.button')}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent className="max-h-[85vh] overflow-y-auto">
        <AlertDialogHeader>
          <AlertDialogTitle>{t('transcribe.title')}</AlertDialogTitle>
          <AlertDialogDescription>{t('transcribe.description')}</AlertDialogDescription>
        </AlertDialogHeader>
        <div className="space-y-4 py-4">
          <LanguageSelector languages={languages} onChange={setLanguages} />
          <div className="flex items-center gap-2">
            <Checkbox
              id="diarization"
              checked={diarization}
              onCheckedChange={(checked) => setDiarization(checked === true)}
            />
            <Label htmlFor="diarization">{t('transcribe.diarization')}</Label>
          </div>
          {diarization && (
            <ExpectedSpeakersSelector
              value={expectedSpeakers}
              onChange={setExpectedSpeakers}
              idPrefix="media"
              disabled={createMutation.isPending}
            />
          )}
          {features?.enhance_audio && (
            <div className="space-y-1.5">
              <div className="flex items-center gap-2">
                <Checkbox
                  id="enhance-audio"
                  checked={enhanceAudio}
                  onCheckedChange={(checked) => setEnhanceAudio(checked === true)}
                />
                <Label htmlFor="enhance-audio">{t('transcribe.enhanceAudio')}</Label>
              </div>
              <p className="text-xs text-muted-foreground ml-6">{t('transcribe.enhanceHint')}</p>
            </div>
          )}
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={createMutation.isPending}>
            {t('common:actions.cancel')}
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={handleSubmit}
            disabled={createMutation.isPending || languages.length === 0}
          >
            {createMutation.isPending ? t('transcribe.starting') : t('transcribe.start')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
