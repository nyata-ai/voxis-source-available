import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { UserStorageQuota } from '@/types/usage'
import { cn, formatBytes } from '@/lib/utils'

interface StorageQuotaNoticeProps {
  storage?: UserStorageQuota
  className?: string
}

export function StorageQuotaNotice({ storage, className }: StorageQuotaNoticeProps) {
  const { t } = useTranslation('media')
  if (!storage?.enforced || (storage.state !== 'warning' && storage.state !== 'full')) return null

  const values = {
    used: formatBytes(storage.used_bytes),
    limit: formatBytes(storage.limit_bytes),
    remaining: formatBytes(storage.remaining_bytes),
  }
  const full = storage.state === 'full'

  return (
    <div
      className={cn(
        'rounded-md border p-3 text-sm',
        full ? 'border-destructive/30 bg-destructive/5 text-destructive' : 'border-warning/30 bg-warning/10 text-warning',
        className,
      )}
      role={full ? 'alert' : 'status'}
    >
      <p className="font-medium">
        {full
          ? t('storageQuota.full', values)
          : t('storageQuota.warning', values)}
      </p>
      {full && (
        <Link to="/library" className="mt-1 inline-block text-xs font-medium underline underline-offset-2">
          {t('storageQuota.manageLibrary')}
        </Link>
      )}
    </div>
  )
}
