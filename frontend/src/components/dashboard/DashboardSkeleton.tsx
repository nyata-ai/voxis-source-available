import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'

/**
 * Drawn to the returning-user Desk (the common case): same 1024px width, same
 * section order and the same heights — header, Continue card, intake strip,
 * five 44px Recent rows, This month — so nothing jumps when data lands.
 */
export function DashboardSkeleton() {
  const { t } = useTranslation('auth')

  return (
    <div className="page-standard" role="status">
      <div className="space-y-3">
        <Skeleton className="h-4 w-32" />
        <Skeleton className="h-8 w-72 max-w-full" />
      </div>
      <div className="page-stack mt-6">
        <Skeleton className="h-44 w-full rounded-lg" />
        <div>
          <Skeleton className="mb-3 h-4 w-36" />
          <div className="grid grid-cols-1 divide-y divide-border/60 overflow-hidden rounded-lg border border-border bg-card md:grid-cols-3 md:divide-x md:divide-y-0">
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="flex h-18 items-center gap-3 px-4">
                <Skeleton className="h-10 w-10 shrink-0 rounded-full" />
                <div className="flex-1 space-y-2">
                  <Skeleton className="h-3.5 w-20" />
                  <Skeleton className="h-3 w-32 max-w-full" />
                </div>
              </div>
            ))}
          </div>
        </div>
        <div>
          <Skeleton className="mb-3 h-4 w-16" />
          <div className="overflow-hidden rounded-lg border border-border bg-card">
            {Array.from({ length: 5 }).map((_, i) => (
              <SkeletonRow key={i} />
            ))}
          </div>
        </div>
        <div>
          <Skeleton className="mb-3 h-4 w-24" />
          <div className="grid grid-cols-2 divide-x divide-border/60 overflow-hidden rounded-lg border border-border bg-card">
            {Array.from({ length: 2 }).map((_, i) => (
              <div key={i} className="space-y-2 px-4 py-5 sm:px-6">
                <Skeleton className="h-3 w-16" />
                <Skeleton className="h-7 w-24" />
                <Skeleton className="h-3 w-32 max-w-full" />
              </div>
            ))}
          </div>
        </div>
      </div>
      <span className="sr-only">{t('dashboard.loading')}</span>
    </div>
  )
}

/** A 44px placeholder shaped like SessionRow: disc, two short lines, duration. */
export function SkeletonRow() {
  return (
    <div className="flex h-11 items-center gap-3 border-b border-border/60 px-3 last:border-b-0">
      <Skeleton className="h-6 w-6 shrink-0 rounded-full" />
      <div className="flex-1 space-y-1.5">
        <Skeleton className="h-3 w-2/5" />
        <Skeleton className="h-2.5 w-1/4" />
      </div>
      <Skeleton className="h-3 w-10" />
    </div>
  )
}
