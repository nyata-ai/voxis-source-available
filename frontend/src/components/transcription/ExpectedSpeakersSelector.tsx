import { useTranslation } from 'react-i18next'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

/**
 * Largest speaker count offered. Mirrors domain.MaxExpectedSpeakers on the
 * backend, which rejects anything higher with a 400.
 */
export const MAX_EXPECTED_SPEAKERS = 10

/** Sentinel for "let the provider decide" — Radix Select forbids an empty value. */
const AUTO_VALUE = 'auto'

interface ExpectedSpeakersSelectorProps {
  /** Declared speaker count; 0 means auto-detect. */
  value: number
  onChange: (value: number) => void
  /** Element id prefix, so two forms can mount this on one page. */
  idPrefix: string
  disabled?: boolean
}

/**
 * Optional "Number of speakers" picker shared by the media transcribe dialog
 * while diarization is enabled. Callers mount it only while diarization is
 * on — a declared count is meaningless without speaker separation, and the
 * backend ignores it in that case.
 */
export function ExpectedSpeakersSelector({
  value,
  onChange,
  idPrefix,
  disabled = false,
}: ExpectedSpeakersSelectorProps) {
  const { t } = useTranslation('transcription')
  const triggerId = `${idPrefix}-expected-speakers`
  const label = t('expectedSpeakers.label')

  return (
    <div className="space-y-1.5">
      <Label htmlFor={triggerId}>{label}</Label>
      <Select
        value={value > 0 ? String(value) : AUTO_VALUE}
        onValueChange={(next) => onChange(next === AUTO_VALUE ? 0 : Number(next))}
        disabled={disabled}
      >
        <SelectTrigger id={triggerId} aria-label={label}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={AUTO_VALUE}>{t('expectedSpeakers.auto')}</SelectItem>
          {Array.from({ length: MAX_EXPECTED_SPEAKERS }, (_, index) => index + 1).map((count) => (
            <SelectItem key={count} value={String(count)}>
              {t('expectedSpeakers.count', { count })}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{t('expectedSpeakers.hint')}</p>
    </div>
  )
}
