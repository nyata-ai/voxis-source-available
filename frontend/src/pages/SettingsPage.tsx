import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { ProfileSection } from '@/components/settings/ProfileSection'
import { AppearanceSection } from '@/components/settings/AppearanceSection'
import { LanguageSection } from '@/components/settings/LanguageSection'
import { TranscriptionSection } from '@/components/settings/TranscriptionSection'
import { SummarySection } from '@/components/settings/SummarySection'
import { PlaybackSection } from '@/components/settings/PlaybackSection'
import { ApiKeysSection } from '@/components/settings/ApiKeysSection'
import { Page, PageHeader } from '@/components/layout/Page'
import { cn } from '@/lib/utils'

export function SettingsPage() {
  const { t } = useTranslation('settings')
  // The open section lives in `?section=`, so Back, refresh and links from the
  // guide land on it. An unknown or absent value means Profile.
  const [searchParams, setSearchParams] = useSearchParams()

  const sections = [
    { id: 'profile', labelKey: 'nav.profile', component: ProfileSection },
    { id: 'appearance', labelKey: 'nav.appearance', component: AppearanceSection },
    { id: 'language', labelKey: 'nav.language', component: LanguageSection },
    { id: 'transcription', labelKey: 'nav.transcription', component: TranscriptionSection },
    { id: 'summary', labelKey: 'nav.summary', component: SummarySection },
    { id: 'playback', labelKey: 'nav.playback', component: PlaybackSection },
    { id: 'api-keys', labelKey: 'nav.apiKeys', component: ApiKeysSection },
  ]

  const requested = searchParams.get('section')
  const active = sections.find((s) => s.id === requested) ?? sections[0]
  const ActiveComponent = active.component

  function openSection(id: string) {
    const next = new URLSearchParams(searchParams)
    if (id === sections[0].id) next.delete('section')
    else next.set('section', id)
    // A section is a view of one page: Back leaves Settings, not the last tab.
    setSearchParams(next, { replace: true })
  }

  return (
    <Page header={<PageHeader title={t('page.title')} description={t('page.subtitle')} />}>
      <div className="flex flex-col gap-6 md:flex-row md:gap-8">
        {/* Phones: one horizontally scrolling row of 44px tabs. md+: a list
            pinned under the header, marked like the sidebar. */}
        <nav aria-label={t('page.title')} className="-mx-6 shrink-0 md:sticky md:top-20 md:mx-0 md:w-48 md:self-start lg:w-56">
          <ul className="flex gap-1 overflow-x-auto px-6 md:flex-col md:gap-px md:overflow-visible md:px-0">
            {sections.map((section) => {
              const isActive = section.id === active.id
              return (
                <li key={section.id} className="shrink-0">
                  <button
                    type="button"
                    onClick={() => openSection(section.id)}
                    aria-current={isActive ? 'page' : undefined}
                    className={cn(
                      'flex min-h-11 w-full items-center whitespace-nowrap border-b-2 px-3 text-left text-body transition-colors md:min-h-9 md:border-b-0 md:border-l-2',
                      'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
                      isActive
                        ? 'border-coral-mark bg-accent/40 text-foreground'
                        : 'border-transparent text-muted-foreground hover:bg-accent/25 hover:text-foreground'
                    )}
                  >
                    {t(section.labelKey)}
                  </button>
                </li>
              )
            })}
          </ul>
        </nav>

        <div className="min-w-0 flex-1">
          <ActiveComponent />
        </div>
      </div>
    </Page>
  )
}
