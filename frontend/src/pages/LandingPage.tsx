import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

export function LandingPage() {
  const { t } = useTranslation('oss')
  return (
    <main className="min-h-screen bg-background text-foreground">
      <header className="mx-auto flex max-w-6xl items-center justify-between px-6 py-6">
        <span className="text-sm font-semibold uppercase tracking-[0.18em]">Voxis</span>
        <div className="flex items-center gap-3">
          <Button asChild variant="ghost">
            <Link to="/guide">{t('landing.guide')}</Link>
          </Button>
          <Button asChild>
            <Link to="/login">{t('landing.signIn')}</Link>
          </Button>
        </div>
      </header>
      <div className="mx-auto max-w-6xl px-6 py-20">
        <p className="text-sm font-medium text-primary">{t('landing.eyebrow')}</p>
        <h1 className="mt-4 max-w-3xl text-4xl font-semibold tracking-tight sm:text-6xl">
          {t('landing.title')}
        </h1>
        <p className="mt-6 max-w-2xl text-lg leading-8 text-muted-foreground">
          {t('landing.summary')}
        </p>
        <div className="mt-8 flex flex-wrap gap-3">
          <Button asChild size="lg">
            <Link to="/login">{t('landing.signIn')}</Link>
          </Button>
          <Button asChild variant="outline" size="lg">
            <Link to="/guide">{t('landing.guide')}</Link>
          </Button>
          <Button asChild variant="outline" size="lg">
            <a href="https://github.com/nyata-ai/voxis-source-available" rel="noreferrer">
              {t('landing.setup')}
            </a>
          </Button>
        </div>
        <div className="mt-16 grid gap-5 md:grid-cols-3">
          <Info title={t('landing.audioTitle')} body={t('landing.audioBody')} />
          <Info title={t('landing.aiTitle')} body={t('landing.aiBody')} />
          <Info title={t('landing.securityTitle')} body={t('landing.securityBody')} />
        </div>
        <section className="mt-12 max-w-4xl rounded-lg border bg-card p-6">
          <h2 className="text-xl font-semibold">{t('landing.licenseTitle')}</h2>
          <p className="mt-3 text-muted-foreground">{t('landing.licenseBody')}</p>
          <p className="mt-3 text-sm text-muted-foreground">{t('landing.pdfLimit')}</p>
        </section>
      </div>
      <footer className="border-t">
        <div className="mx-auto flex max-w-6xl flex-wrap gap-4 px-6 py-6 text-sm text-muted-foreground">
          <Link to="/terms">{t('landing.terms')}</Link>
          <Link to="/privacy">{t('landing.security')}</Link>
          <a href="mailto:oss@nyata.ai">oss@nyata.ai</a>
        </div>
      </footer>
    </main>
  )
}

function Info({ title, body }: { title: string; body: string }) {
  return (
    <section className="rounded-lg border bg-card p-5">
      <h2 className="font-semibold">{title}</h2>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">{body}</p>
    </section>
  )
}
