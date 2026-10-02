import { Headphones, Mic } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Label } from '@/components/ui/label'
import type { RecordingCaptureSource } from '@/types/recording'

interface CaptureSourceSelectorProps {
  value: RecordingCaptureSource
  onChange: (value: RecordingCaptureSource) => void
  disabled?: boolean
}

export function CaptureSourceSelector({
  value,
  onChange,
  disabled = false,
}: CaptureSourceSelectorProps) {
  const { t } = useTranslation('recording')
  const hasDisplayCapture =
    typeof navigator !== 'undefined' &&
    typeof navigator.mediaDevices?.getDisplayMedia === 'function'
  const mixedDisabled = disabled || !hasDisplayCapture

  return (
    <div className="w-full max-w-md space-y-2 text-left">
      <Label>{t('captureSource.label')}</Label>
      <RadioGroup
        value={value}
        onValueChange={(next) => onChange(next as RecordingCaptureSource)}
        className="grid grid-cols-2 gap-2"
        aria-label={t('captureSource.label')}
        disabled={disabled}
      >
        <Label
          htmlFor="capture-source-microphone"
          className="flex min-h-12 cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm"
        >
          <RadioGroupItem id="capture-source-microphone" value="microphone" />
          <Mic className="h-4 w-4 text-muted-foreground" />
          <span>{t('captureSource.microphone')}</span>
        </Label>
        <Label
          htmlFor="capture-source-mixed"
          className="flex min-h-12 cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-60"
        >
          <RadioGroupItem
            id="capture-source-mixed"
            value="mixed_audio"
            disabled={mixedDisabled}
          />
          <Headphones className="h-4 w-4 text-muted-foreground" />
          <span>{t('captureSource.mixed')}</span>
        </Label>
      </RadioGroup>
      {value === 'mixed_audio' && hasDisplayCapture && (
        <p className="text-xs text-muted-foreground">{t('captureSource.mixedHelp')}</p>
      )}
      {!hasDisplayCapture && (
        <p className="text-xs text-muted-foreground">{t('captureSource.mixedUnsupported')}</p>
      )}
    </div>
  )
}
