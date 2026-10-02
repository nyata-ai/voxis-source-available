import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { usePreferences, useUpdatePreferences } from '@/hooks/useSettings'
import { Label } from '@/components/ui/label'
import { Slider } from '@/components/ui/slider'
import { Skeleton } from '@/components/ui/skeleton'

export function PlaybackSection() {
  const { t } = useTranslation('settings')
  const { data: preferences, isLoading } = usePreferences()
  const { mutate: updatePreferences } = useUpdatePreferences()
  const [localSpeed, setLocalSpeed] = useState<number | null>(null)

  if (isLoading || !preferences) {
    return (
      <div className="space-y-6">
        <div>
          <h3 className="text-lg font-medium">{t('playback.title')}</h3>
          <p className="text-sm text-muted-foreground">{t('playback.description')}</p>
        </div>
        <div className="space-y-4 rounded-lg border p-4">
          <Skeleton className="h-6 w-32" />
          <Skeleton className="h-4 w-full" />
        </div>
      </div>
    )
  }

  const displaySpeed = localSpeed ?? preferences.playback_speed

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-lg font-medium">{t('playback.title')}</h3>
        <p className="text-sm text-muted-foreground">{t('playback.description')}</p>
      </div>

      <div className="space-y-4 rounded-lg border p-4">
        <div className="flex items-center justify-between">
          <Label>{t('playback.speed')}</Label>
          <span className="text-sm font-medium">{Number.isInteger(displaySpeed) ? displaySpeed.toFixed(1) : String(displaySpeed)}x</span>
        </div>
        <Slider
          min={0.5}
          max={2}
          step={0.25}
          value={[displaySpeed]}
          onValueChange={([value]) => setLocalSpeed(value)}
          onValueCommit={([value]) => {
            setLocalSpeed(null)
            updatePreferences({ playback_speed: value })
          }}
        />
        <div className="flex justify-between text-xs text-muted-foreground">
          <span>0.5x</span>
          <span>1.0x</span>
          <span>1.5x</span>
          <span>2.0x</span>
        </div>
      </div>
    </div>
  )
}
