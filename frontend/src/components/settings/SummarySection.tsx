import { useTranslation } from 'react-i18next'
import { useFeatures } from '@/hooks/useFeatures'
import { usePreferences, useUpdatePreferences } from '@/hooks/useSettings'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  normalizeSummaryProfile,
  SUMMARY_PROFILES,
  type SummaryProfile,
  type UserPreferences,
} from '@/types/settings'

const SUMMARY_TYPES: { value: UserPreferences['default_summary_type']; labelKey: string }[] = [
  { value: 'general', labelKey: 'summary.types.general' },
  { value: 'key_points', labelKey: 'summary.types.keyPoints' },
  { value: 'action_items', labelKey: 'summary.types.actionItems' },
  { value: 'q_and_a', labelKey: 'summary.types.qAndA' },
]

const EXPORT_FORMATS: { value: UserPreferences['default_export_format']; label: string }[] = [
  { value: 'pdf', label: 'PDF' },
  { value: 'docx', label: 'DOCX' },
  { value: 'json', label: 'JSON' },
]

interface SummaryProfileSelectProps {
  value: SummaryProfile
  onValueChange: (profile: SummaryProfile) => void
}

function SummaryProfileSelect({ value, onValueChange }: SummaryProfileSelectProps) {
  const { t } = useTranslation('settings')

  return (
    <div className="space-y-3">
      <Label htmlFor="summary-profile">{t('summary.profile.label')}</Label>
      <Select value={value} onValueChange={(profile) => onValueChange(profile as SummaryProfile)}>
        <SelectTrigger id="summary-profile" aria-describedby="summary-profile-description">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {SUMMARY_PROFILES.map((profile) => (
            <SelectItem key={profile} value={profile}>
              {t(`summary.profile.options.${profile}.label`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p id="summary-profile-description" className="text-xs text-muted-foreground">
        {t('summary.profile.description')}
      </p>
      <p className="text-xs text-muted-foreground">
        {t(`summary.profile.options.${value}.description`)}
      </p>
    </div>
  )
}

export function SummarySection() {
  const { t } = useTranslation('settings')
  const { data: features } = useFeatures()
  const { data: preferences, isLoading } = usePreferences()
  const { mutate: updatePreferences } = useUpdatePreferences()
  const summaryProfilesEnabled = features?.summary_profiles === true

  if (isLoading || !preferences) {
    return (
      <div className="space-y-6">
        <div>
          <h3 className="text-lg font-medium">{t('summary.title')}</h3>
          <p className="text-sm text-muted-foreground">{t('summary.description')}</p>
        </div>
        <div className="space-y-4 rounded-lg border p-4">
          <Skeleton className="h-6 w-32" />
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-6 w-32" />
          <Skeleton className="h-8 w-full" />
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-lg font-medium">{t('summary.title')}</h3>
        <p className="text-sm text-muted-foreground">{t('summary.description')}</p>
      </div>

      <div className="space-y-6 rounded-lg border p-4">
        {summaryProfilesEnabled && (
          <SummaryProfileSelect
            value={normalizeSummaryProfile(preferences.summary_profile)}
            onValueChange={(summary_profile) => updatePreferences({ summary_profile })}
          />
        )}

        <div className="space-y-3">
          <Label>{t('summary.defaultType')}</Label>
          <RadioGroup
            value={preferences.default_summary_type}
            onValueChange={(value) =>
              updatePreferences({
                default_summary_type: value as UserPreferences['default_summary_type'],
              })
            }
          >
            {SUMMARY_TYPES.map(({ value, labelKey }) => (
              <div key={value} className="flex items-center space-x-2">
                <RadioGroupItem value={value} id={`summary-${value}`} aria-label={t(labelKey)} />
                <Label htmlFor={`summary-${value}`} className="font-normal cursor-pointer">
                  {t(labelKey)}
                </Label>
              </div>
            ))}
          </RadioGroup>
          <p className="text-xs text-muted-foreground">{t('summary.typeDescription')}</p>
        </div>

        <div className="space-y-3">
          <Label>{t('summary.defaultFormat')}</Label>
          <RadioGroup
            value={preferences.default_export_format}
            onValueChange={(value) =>
              updatePreferences({
                default_export_format: value as UserPreferences['default_export_format'],
              })
            }
          >
            {EXPORT_FORMATS.map(({ value, label }) => (
              <div key={value} className="flex items-center space-x-2">
                <RadioGroupItem value={value} id={`format-${value}`} aria-label={label} />
                <Label htmlFor={`format-${value}`} className="font-normal cursor-pointer">
                  {label}
                </Label>
              </div>
            ))}
          </RadioGroup>
          <p className="text-xs text-muted-foreground">{t('summary.formatDescription')}</p>
        </div>

        <div className="flex items-center justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="auto-briefings">{t('summary.autoBriefings.label')}</Label>
            <p className="text-xs text-muted-foreground">
              {t('summary.autoBriefings.description')}
            </p>
          </div>
          <Switch
            id="auto-briefings"
            checked={preferences.auto_briefings === true}
            onCheckedChange={(checked) => updatePreferences({ auto_briefings: checked })}
            aria-label={t('summary.autoBriefings.label')}
          />
        </div>

        <div className="flex items-center justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="high-stakes-summaries">{t('summary.highStakes.label')}</Label>
            <p className="text-xs text-muted-foreground">{t('summary.highStakes.description')}</p>
          </div>
          <Switch
            id="high-stakes-summaries"
            checked={preferences.high_stakes_summaries}
            onCheckedChange={(checked) => updatePreferences({ high_stakes_summaries: checked })}
            aria-label={t('summary.highStakes.label')}
          />
        </div>
      </div>
    </div>
  )
}
