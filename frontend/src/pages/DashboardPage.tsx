import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ActionTriad } from '@/components/dashboard/ActionTriad'
import { InMotionBand } from '@/components/activity/InMotionBand'
import { Button } from '@/components/ui/button'
import { useCurrentUser } from '@/hooks/useCurrentUser'
import { useDashboardStats } from '@/hooks/useDashboard'

export function DashboardPage() {
  const { t } = useTranslation('oss')
  const user = useCurrentUser()
  const stats = useDashboardStats()
  const name = user.data?.name || user.data?.email?.split('@')[0] || t('dashboard.defaultName')
  return (
    <div className="mx-auto max-w-5xl space-y-8">
      <header>
        <p className="text-sm text-muted-foreground">{user.data?.organization?.name || 'Voxis'}</p>
        <h1 className="mt-1 text-3xl font-semibold">{t('dashboard.welcome', { name })}</h1>
        <p className="mt-2 max-w-2xl text-muted-foreground">{t('dashboard.dataFlow')}</p>
      </header>
      <ActionTriad />
      <InMotionBand />
      <section className="rounded-lg border bg-card p-6">
        <h2 className="text-lg font-medium">{t('dashboard.libraryTitle')}</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          {stats.isError
            ? t('dashboard.statsUnavailable')
            : t('dashboard.libraryBody', { files: stats.data?.total_media ?? 0 })}
        </p>
        <Button asChild variant="outline" className="mt-4">
          <Link to="/library">{t('dashboard.openLibrary')}</Link>
        </Button>
      </section>
    </div>
  )
}
