import { useTranslation } from 'react-i18next'
import { ArrowDown } from 'lucide-react'
import { HeroWaveform } from './HeroWaveform'

interface WelcomeHeroProps {
  displayName: string
}

/** First-visit welcome: the Desk's one coral surface, sitting above the three
 * intake tiles and handing over to them with "Start with one of these". */
export function WelcomeHero({ displayName }: WelcomeHeroProps) {
  const { t } = useTranslation('auth')

  return (
    <section aria-labelledby="welcome-heading">
      <div className="overflow-hidden rounded-lg bg-coral text-coral-foreground">
        <div className="grid gap-0 md:grid-cols-[minmax(0,1fr)_minmax(0,1.6fr)]">
          <div className="hidden items-center overflow-hidden border-coral-foreground/15 bg-white/10 px-6 md:flex md:border-r">
            <HeroWaveform seed={`welcome-${displayName}`} className="text-coral-foreground/60" />
          </div>
          <div className="flex flex-col gap-4 p-6 sm:p-8">
            <p className="kicker text-coral-foreground/85">{t('dashboard.welcome.eyebrow')}</p>
            <h2 id="welcome-heading" className="text-page font-normal text-coral-foreground sm:text-display">
              {t('dashboard.welcome.title', { name: displayName })}
            </h2>
            <p className="max-w-md text-body text-coral-foreground/85">{t('dashboard.welcome.body')}</p>
            <p className="text-body text-coral-foreground/85">{t('dashboard.welcome.security')}</p>
            <p className="flex items-center gap-1.5 text-body font-semibold text-coral-foreground">
              {t('dashboard.welcome.startWith')}
              <ArrowDown className="h-4 w-4" aria-hidden="true" />
            </p>
          </div>
        </div>
      </div>
    </section>
  )
}
