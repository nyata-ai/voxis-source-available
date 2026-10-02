import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ProfileSection } from '@/components/settings/ProfileSection'
import { AppearanceSection } from '@/components/settings/AppearanceSection'
import { LanguageSection } from '@/components/settings/LanguageSection'
import { TranscriptionSection } from '@/components/settings/TranscriptionSection'
import { SummarySection } from '@/components/settings/SummarySection'
import { PlaybackSection } from '@/components/settings/PlaybackSection'
import { ApiKeysSection } from '@/components/settings/ApiKeysSection'

export function SettingsPage() {
  const { t } = useTranslation('settings')
  const [activeSection, setActiveSection] = useState('profile')

  const sections = [
    { id: 'profile', labelKey: 'nav.profile', component: ProfileSection },
    { id: 'appearance', labelKey: 'nav.appearance', component: AppearanceSection },
    { id: 'language', labelKey: 'nav.language', component: LanguageSection },
    { id: 'transcription', labelKey: 'nav.transcription', component: TranscriptionSection },
    { id: 'summary', labelKey: 'nav.summary', component: SummarySection },
    { id: 'playback', labelKey: 'nav.playback', component: PlaybackSection },
    { id: 'api-keys', labelKey: 'nav.apiKeys', component: ApiKeysSection },
  ]

  const ActiveComponent = sections.find((s) => s.id === activeSection)?.component ?? ProfileSection

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-normal">{t('page.title')}</h1>
        <p className="text-muted-foreground">{t('page.subtitle')}</p>
      </div>

      <div className="flex flex-col gap-8 md:flex-row">
        <nav className="w-full md:w-48 lg:w-56 shrink-0">
          <ul className="space-y-1">
            {sections.map((section) => (
              <li key={section.id}>
                <button
                  onClick={() => setActiveSection(section.id)}
                  className={`w-full rounded-md px-3 py-2 text-left text-sm font-medium transition-colors ${
                    activeSection === section.id
                      ? 'bg-muted text-foreground'
                      : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground'
                  }`}
                >
                  {t(section.labelKey)}
                </button>
              </li>
            ))}
          </ul>
        </nav>

        <div className="flex-1 min-w-0">
          <ActiveComponent />
        </div>
      </div>
    </div>
  )
}
