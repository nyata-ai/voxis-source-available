import { useTranslation } from 'react-i18next'
import { Globe } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { TRANSLATED_LANGUAGE_CODE_SET, pickerLocales, DEFAULT_LOCALE } from '@/i18n/config'
import { changeAppLanguage } from '@/i18n/language'
import { useUpdatePreferences } from '@/hooks/useSettings'
import { toast } from '@/lib/toast'

export function LanguageSwitcher() {
  const { t, i18n } = useTranslation('common')
  const { mutate: updatePreferences } = useUpdatePreferences()
  // Equivalent to currentLanguage(): same singleton, same precedence. Reactivity
  // is not the reason for either — useTranslation subscribes to `languageChanged`
  // and re-renders this component whichever one reads the value.
  const active = i18n.resolvedLanguage ?? i18n.language ?? DEFAULT_LOCALE
  const LOCALES = pickerLocales(active)
  const current = LOCALES.find((l) => l.code === active) ?? LOCALES[0]

  function handleChange(code: string) {
    // The `code === active` half is not just an optimisation: persisting is
    // one-way (the backend rejects an empty ui_language), so a no-op click must
    // never become a permanent opt-out of detection.
    if (code === active || !TRANSLATED_LANGUAGE_CODE_SET.has(code)) return
    void changeAppLanguage(code)
    // Persist server-side so the choice survives device changes (mirrors LanguageSection).
    // A failure has to be said out loud: the switch already applied locally, so the
    // only symptom is the next refetch handing the stale server value back to
    // useSyncLanguage — the UI silently reverting on every reload.
    updatePreferences(
      { ui_language: code },
      { onError: () => toast.error(t('language.saveFailed')) }
    )
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={t('language.title')}
          className="flex h-8 items-center gap-1.5 border bg-card/60 px-2.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        >
          <Globe className="h-3.5 w-3.5" aria-hidden="true" />
          <span>{current.shortCode}</span>
        </button>
      </DropdownMenuTrigger>

      <DropdownMenuContent align="end" sideOffset={8} className="w-52">
        <DropdownMenuRadioGroup value={active} onValueChange={handleChange}>
          {LOCALES.map(({ code, shortCode, nativeLabel }) => (
            <DropdownMenuRadioItem key={code} value={code} className="gap-2">
              <span className="w-7 shrink-0 text-xs font-medium text-muted-foreground">
                {shortCode}
              </span>
              <span>{nativeLabel}</span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
