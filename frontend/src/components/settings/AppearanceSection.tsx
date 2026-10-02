import { Sun, Moon, Monitor } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useThemeStore, type Theme } from '@/stores/theme'
import { useUpdatePreferences } from '@/hooks/useSettings'
import { cn } from '@/lib/utils'

const themes: { value: Theme; labelKey: string; icon: typeof Sun; descriptionKey: string }[] = [
  { value: 'light', labelKey: 'appearance.light', icon: Sun, descriptionKey: 'appearance.lightDescription' },
  { value: 'dark', labelKey: 'appearance.dark', icon: Moon, descriptionKey: 'appearance.darkDescription' },
  { value: 'system', labelKey: 'appearance.system', icon: Monitor, descriptionKey: 'appearance.systemDescription' },
]

export function AppearanceSection() {
  const { t } = useTranslation('settings')
  const { theme, setTheme } = useThemeStore()
  const { mutate: updatePreferences } = useUpdatePreferences()

  function handleThemeChange(newTheme: Theme) {
    setTheme(newTheme)
    updatePreferences({ theme: newTheme })
  }

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-lg font-medium">{t('appearance.title')}</h3>
        <p className="text-sm text-muted-foreground">{t('appearance.description')}</p>
      </div>

      <div className="grid grid-cols-3 gap-4">
        {themes.map(({ value, labelKey, icon: Icon, descriptionKey }) => {
          const isActive = theme === value
          return (
            <button
              key={value}
              type="button"
              onClick={() => handleThemeChange(value)}
              className={cn(
                'flex flex-col items-center gap-2 rounded-lg border p-4 transition-colors hover:bg-accent',
                isActive && 'ring-2 ring-primary border-primary',
              )}
            >
              <Icon className="h-6 w-6" />
              <span className="text-sm font-medium">{t(labelKey)}</span>
              <span className="text-xs text-muted-foreground text-center">{t(descriptionKey)}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
