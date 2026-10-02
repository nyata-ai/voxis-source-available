import { useEffect } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/contexts/AuthContext'
import { supportsPkce } from '@/lib/keycloak'

export function LoginPage() {
  const { t } = useTranslation('oss')
  const { login, isAuthenticated, isLoading, error } = useAuth()
  useEffect(() => {
    if (isAuthenticated) window.location.assign('/dashboard')
  }, [isAuthenticated])
  return (
    <main className="flex min-h-screen items-center justify-center p-6">
      <section className="w-full max-w-md rounded-lg border bg-card p-8">
        <p className="text-sm font-semibold uppercase tracking-[0.18em]">Voxis Source-Available</p>
        <h1 className="mt-4 text-2xl font-semibold">{t('login.title')}</h1>
        <p className="mt-2 text-muted-foreground">{t('login.body')}</p>
        {!supportsPkce && (
          <p className="mt-4 text-sm text-destructive">{t('login.secureContext')}</p>
        )}
        {error && <p className="mt-4 text-sm text-destructive">{error}</p>}
        <Button
          className="mt-6 w-full"
          onClick={() => void login()}
          disabled={isLoading || !supportsPkce}
        >
          {t('login.button')}
        </Button>
        <p className="mt-5 text-sm text-muted-foreground">{t('login.accountNote')}</p>
        <Link className="mt-4 inline-block text-sm underline" to="/">
          {t('login.back')}
        </Link>
      </section>
    </main>
  )
}
