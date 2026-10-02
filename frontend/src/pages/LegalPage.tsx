import { Link, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'

export function LegalPage() {
  const { t } = useTranslation('oss')
  const privacy = useLocation().pathname === '/privacy'
  return (
    <main className="mx-auto min-h-screen max-w-3xl px-6 py-16">
      <Link to="/" className="text-sm underline">
        Voxis Source-Available
      </Link>
      {privacy ? <DataFlow t={t} /> : <Terms t={t} />}
    </main>
  )
}

type Translate = (key: string) => string

function Terms({ t }: { t: Translate }) {
  return (
    <article className="prose mt-8 max-w-none">
      <h1>{t('legal.termsTitle')}</h1>
      <p>{t('legal.sourceAvailable')}</p>
      <p>{t('legal.allowedUse')}</p>
      <p>{t('legal.evaluation')}</p>
    </article>
  )
}
function DataFlow({ t }: { t: Translate }) {
  return (
    <article className="prose mt-8 max-w-none">
      <h1>{t('legal.dataTitle')}</h1>
      <p>{t('legal.dataFlow')}</p>
      <p>{t('legal.boundary')}</p>
      <p>{t('legal.retention')}</p>
    </article>
  )
}
