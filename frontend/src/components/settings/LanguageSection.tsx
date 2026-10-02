import { useTranslation } from 'react-i18next'
import { TRANSLATED_LANGUAGE_CODE_SET, pickerLocales } from '@/i18n/config'
import { changeAppLanguage, currentLanguage } from '@/i18n/language'
import { useUpdatePreferences } from '@/hooks/useSettings'
import { toast } from '@/lib/toast'
import { cn } from '@/lib/utils'

export function LanguageSection() {
  const { t } = useTranslation('common')
  const { mutate: updatePreferences } = useUpdatePreferences()
  const active = currentLanguage()

  function handleLanguageChange(code: string) {
    // Deliberately NO `code === active` guard, unlike LanguageSwitcher. This is a
    // configuration surface, so clicking a language here is a statement of intent
    // even when it is the one already on screen — and since persisting is one-way
    // (the backend rejects an empty ui_language), it is the only way to pin a
    // detected language. Swallowing the click would leave a reader who was
    // correctly detected as Indonesian with no way to make that stick. The quick
    // dropdown in the header is where a no-op click really is a no-op.
    if (!TRANSLATED_LANGUAGE_CODE_SET.has(code)) return
    void changeAppLanguage(code)
    // A failure has to be said out loud: the switch already applied locally, so the
    // only symptom is the next refetch handing the stale server value back to
    // useSyncLanguage — the UI silently reverting on every reload.
    updatePreferences(
      { ui_language: code },
      { onError: () => toast.error(t('language.saveFailed')) }
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-lg font-medium">{t('language.title')}</h3>
        <p className="text-sm text-muted-foreground">{t('language.description')}</p>
      </div>

      <div className="grid grid-cols-3 gap-4">
        {pickerLocales(active).map(({ code, label, nativeLabel }) => {
          const isAvailable = TRANSLATED_LANGUAGE_CODE_SET.has(code)
          const isActive = active === code && isAvailable
          return (
            <button
              key={code}
              type="button"
              disabled={!isAvailable}
              aria-pressed={isActive}
              onClick={() => handleLanguageChange(code)}
              className={cn(
                'flex flex-col items-center gap-1 rounded-lg border p-4 transition-colors hover:bg-accent',
                !isAvailable && 'cursor-not-allowed opacity-45 hover:bg-transparent',
                isActive && 'ring-2 ring-primary border-primary'
              )}
            >
              <span className="text-sm font-medium">{nativeLabel}</span>
              <span className="text-xs text-muted-foreground text-center">{label}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
