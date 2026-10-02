import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { LANGUAGE_GROUPS, MAX_LANGUAGES } from '@/lib/languages'
import { cn } from '@/lib/utils'

interface LanguageSelectorProps {
  languages: string[]
  onChange: (languages: string[]) => void
}

export function LanguageSelector({ languages, onChange }: LanguageSelectorProps) {
  const { t } = useTranslation(['transcription', 'common'])
  const isAutoDetect = languages.length === 1 && languages[0] === 'auto'
  const specificCount = isAutoDetect ? 0 : languages.length
  const atMax = specificCount >= MAX_LANGUAGES

  function handleAutoToggle() {
    onChange(['auto'])
  }

  function handleLanguageToggle(code: string) {
    if (isAutoDetect) {
      onChange([code])
      return
    }

    const isSelected = languages.includes(code)
    if (isSelected) {
      const next = languages.filter((l) => l !== code)
      onChange(next.length === 0 ? ['auto'] : next)
    } else {
      if (atMax) return
      onChange([...languages, code])
    }
  }

  return (
    <div className="space-y-3">
      <Label>{t('languageSelector.label')}</Label>
      <p className="text-xs text-muted-foreground">
        {t('languageSelector.hint', { max: MAX_LANGUAGES })}
      </p>
      <Button
        type="button"
        variant={isAutoDetect ? 'default' : 'outline'}
        className="w-full"
        onClick={handleAutoToggle}
      >
        {t('languageSelector.autoDetect')}
      </Button>
      <div className={cn('space-y-3', isAutoDetect && 'opacity-50')}>
        {LANGUAGE_GROUPS.map((group) => (
          <div key={group.key}>
            <p className="text-xs font-medium text-muted-foreground mb-1.5">
              {t(`common:${group.nameKey}`)}
            </p>
            <div className="grid grid-cols-3 gap-1.5">
              {group.languages.map((lang) => {
                const selected = languages.includes(lang.value)
                const disabled = !selected && atMax
                return (
                  <Button
                    key={lang.value}
                    type="button"
                    variant={selected ? 'default' : 'outline'}
                    size="sm"
                    disabled={disabled}
                    onClick={() => handleLanguageToggle(lang.value)}
                    aria-pressed={selected}
                  >
                    {t(`common:${lang.labelKey}`)}
                  </Button>
                )
              })}
            </div>
          </div>
        ))}
      </div>
      {!isAutoDetect && (
        <p className="text-xs text-muted-foreground">
          {t('languageSelector.selectedCount', { selected: specificCount, max: MAX_LANGUAGES })}
        </p>
      )}
    </div>
  )
}
