import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ActionTriad } from '@/components/dashboard/ActionTriad'
import { ContinueCard } from '@/components/dashboard/ContinueCard'
import { buildLibrary, pickContinueItem } from '@/components/dashboard/library'
import { InMotionBand } from '@/components/activity/InMotionBand'
import { PageHeader } from '@/components/layout/Page'
import { Button } from '@/components/ui/button'
import { useCurrentUser } from '@/hooks/useCurrentUser'
import { useDashboardStats } from '@/hooks/useDashboard'
import { useSearchMedia } from '@/hooks/useMedia'
import { useTranscriptionList } from '@/hooks/useTranscription'

export function DashboardPage() {
  const { t } = useTranslation('oss')
  const user = useCurrentUser()
  const stats = useDashboardStats()
  const { data: mediaResponse } = useSearchMedia({ limit: 12 })
  const { data: transcriptionResponse } = useTranscriptionList(15, 0)
  const continueItem = useMemo(
    () =>
      pickContinueItem(
        buildLibrary(mediaResponse?.items ?? [], transcriptionResponse?.items ?? [])
      ),
    [mediaResponse, transcriptionResponse]
  )
  const name = user.data?.name || user.data?.email?.split('@')[0] || t('dashboard.defaultName')

  return (
    <div className="page-standard">
      <PageHeader
        kicker={user.data?.organization?.name || 'Voxis'}
        title={t('dashboard.welcome', { name })}
        description={t('dashboard.dataFlow')}
      />
      <div className="page-stack mt-6">
        {continueItem && <ContinueCard item={continueItem} />}
        <InMotionBand />
        <ActionTriad variant={continueItem ? 'compact' : 'full'} />
        <section className="rounded-lg border bg-card p-6">
          <h2 className="text-section font-normal">{t('dashboard.libraryTitle')}</h2>
          <p className="mt-2 text-body text-muted-foreground">
            {stats.isError
              ? t('dashboard.statsUnavailable')
              : t('dashboard.libraryBody', { files: stats.data?.total_media ?? 0 })}
          </p>
          <Button asChild variant="outline" className="mt-4">
            <Link to="/library">{t('dashboard.openLibrary')}</Link>
          </Button>
        </section>
      </div>
    </div>
  )
}
