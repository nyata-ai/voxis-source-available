import { useTranslation } from 'react-i18next'
import { ExternalLink } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { StorageQuotaNotice } from '@/components/media/StorageQuotaNotice'
import { useAuth } from '@/contexts/AuthContext'
import { useUsageStats } from '@/hooks/useUsageStats'
import { getKeycloakAccountSecurityUrl } from '@/lib/keycloak'

export function ProfileSection() {
  const { t } = useTranslation('settings')
  const { user, isLoading } = useAuth()
  const usage = useUsageStats()
  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-lg font-medium">{t('profile.title')}</h3>
        <p className="text-sm text-muted-foreground">{t('profile.description')}</p>
      </div>
      {isLoading || !user ? (
        <div className="space-y-3 rounded-lg border p-4">
          <Skeleton className="h-5 w-48" />
          <Skeleton className="h-4 w-64" />
        </div>
      ) : (
        <div className="space-y-4 rounded-lg border p-4">
          <dl className="grid gap-3 text-sm">
            <div>
              <dt className="text-muted-foreground">{t('profile.name')}</dt>
              <dd>{user.name}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('profile.email')}</dt>
              <dd>{user.email}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('profile.username')}</dt>
              <dd>{user.username}</dd>
            </div>
          </dl>
          <p className="text-xs text-muted-foreground">{t('profile.ssoNote')}</p>
          <Button asChild variant="outline" size="sm">
            <a href={getKeycloakAccountSecurityUrl()} target="_blank" rel="noreferrer">
              {t('profile.securityAction')} <ExternalLink className="ml-1 h-3.5 w-3.5" />
            </a>
          </Button>
        </div>
      )}
      <div>
        <h3 className="text-lg font-medium">{t('profile.usage.title')}</h3>
        <p className="text-sm text-muted-foreground">{t('profile.usage.description')}</p>
      </div>
      {usage.isLoading ? (
        <Skeleton className="h-28 w-full" />
      ) : usage.isError || !usage.data ? (
        <p className="text-sm text-muted-foreground">{t('profile.usage.error')}</p>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          <Metric
            label={t('profile.usage.audioProcessed')}
            value={`${Math.round(usage.data.total_duration_seconds / 60)} min`}
          />
          <Metric
            label={t('profile.usage.aiTokens')}
            value={String(
              usage.data.total_prompt_tokens +
                usage.data.total_completion_tokens +
                usage.data.total_thinking_tokens
            )}
          />
          <div className="rounded-lg border p-4 sm:col-span-2">
            <p className="text-sm font-medium">{t('profile.usage.storage')}</p>
            <StorageQuotaNotice storage={usage.data.user_storage} className="mt-2" />
          </div>
        </div>
      )}
    </div>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border p-4">
      <p className="text-sm text-muted-foreground">{label}</p>
      <p className="mt-1 text-2xl font-semibold">{value}</p>
    </div>
  )
}
