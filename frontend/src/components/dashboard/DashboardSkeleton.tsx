import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'

export function DashboardSkeleton() {
  const { t } = useTranslation('auth')

  return (
    <div className="space-y-10" role="status">
      <div className="space-y-3">
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-7 w-full" />
      </div>
      <Skeleton className="h-44 w-full rounded-lg" />
      <div className="grid grid-cols-1 gap-px overflow-hidden rounded-lg bg-border md:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="space-y-3 bg-card px-6 py-6">
            <Skeleton className="h-3 w-20" />
            <Skeleton className="h-10 w-24" />
            <Skeleton className="h-3 w-32" />
          </div>
        ))}
      </div>
      <div className="space-y-2">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-14 w-full rounded-lg" />
        ))}
      </div>
      <span className="sr-only">{t('dashboard.loading')}</span>
    </div>
  )
}
