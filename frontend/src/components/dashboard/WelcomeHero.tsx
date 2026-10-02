import { useTranslation } from 'react-i18next'
import { HeroWaveform } from './HeroWaveform'

interface WelcomeHeroProps {
  displayName: string
}

export function WelcomeHero({ displayName }: WelcomeHeroProps) {
  const { t } = useTranslation('auth')

  return (
    <section className="animate-[fadeInUp_0.5s_ease-out_both]">
      <div className="overflow-hidden rounded-lg bg-coral text-coral-foreground">
        <div className="grid gap-0 md:grid-cols-[minmax(0,1fr)_minmax(0,1.6fr)]">
          <div className="flex h-44 items-center overflow-hidden border-b border-coral-foreground/15 bg-white/10 px-6 md:h-auto md:border-b-0 md:border-r">
            <HeroWaveform seed={`welcome-${displayName}`} className="text-coral-foreground/60" />
          </div>
          <div className="flex flex-col gap-5 p-6 sm:p-9">
            <p className="text-[0.68rem] font-semibold uppercase tracking-[0.22em] text-coral-foreground/85">
              {t('dashboard.welcome.eyebrow')}
            </p>
            <h2 className="text-[1.85rem] font-normal leading-tight text-coral-foreground sm:text-[2.25rem]">
              {t('dashboard.welcome.title', { name: displayName })}
            </h2>
            <p className="max-w-md text-sm leading-6 text-coral-foreground/85">
              {t('dashboard.welcome.body')}
            </p>
            <p className="text-[0.85rem] text-coral-foreground/85">
              {t('dashboard.welcome.security')}
            </p>
          </div>
        </div>
      </div>
    </section>
  )
}
