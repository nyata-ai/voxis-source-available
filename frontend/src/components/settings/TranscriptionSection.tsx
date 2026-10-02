import { useTranslation } from 'react-i18next'
import { usePreferences, useUpdatePreferences } from '@/hooks/useSettings'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Skeleton } from '@/components/ui/skeleton'
import { LanguageSelector } from '@/components/transcription/LanguageSelector'

export function TranscriptionSection() {
  const { t } = useTranslation('settings')
  const { data: preferences, isLoading } = usePreferences()
  const { mutate: updatePreferences } = useUpdatePreferences()

  if (isLoading || !preferences) {
    return (
      <div className="space-y-6">
        <div>
          <h3 className="text-lg font-medium">{t('transcription.title')}</h3>
          <p className="text-sm text-muted-foreground">{t('transcription.description')}</p>
        </div>
        <div className="space-y-4 rounded-lg border p-4">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-6 w-48" />
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-lg font-medium">{t('transcription.title')}</h3>
        <p className="text-sm text-muted-foreground">{t('transcription.description')}</p>
      </div>

      <div className="space-y-6 rounded-lg border p-4">
        <div>
          <LanguageSelector
            languages={preferences.default_languages}
            onChange={(langs) => updatePreferences({ default_languages: langs })}
          />
          <p className="mt-2 text-xs text-muted-foreground">
            {t('transcription.defaultLanguage')}
          </p>
        </div>

        <div className="flex items-center justify-between">
          <div className="space-y-0.5">
            <Label>{t('transcription.diarization.label')}</Label>
            <p className="text-xs text-muted-foreground">
              {t('transcription.diarization.description')}
            </p>
          </div>
          <Switch
            checked={preferences.default_diarization === true}
            onCheckedChange={(checked) => updatePreferences({ default_diarization: checked })}
          />
        </div>
      </div>
    </div>
  )
}
