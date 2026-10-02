import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useAdminStorageQuota, useUpdateAdminStorageQuota } from '@/hooks/useAdminStorageQuota'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/lib/toast'

const GIB = 1024 ** 3

function parseLimitBytes(input: string): number | null {
  const gibibytes = Number(input)
  if (!Number.isFinite(gibibytes) || gibibytes <= 0) return null
  const bytes = Math.round(gibibytes * GIB)
  if (!Number.isSafeInteger(bytes) || bytes <= 0) return null
  return bytes
}

export function AdminStorageQuotaPanel() {
  const { t } = useTranslation('media')
  const { data: quota, isLoading, isError } = useAdminStorageQuota()
  const updateQuota = useUpdateAdminStorageQuota()
  const [enabled, setEnabled] = useState(false)
  const [limitInput, setLimitInput] = useState('2')
  const [limitChanged, setLimitChanged] = useState(false)

  useEffect(() => {
    if (!quota) return
    setEnabled(quota.enabled)
    setLimitInput(String(quota.default_limit_bytes / GIB))
    setLimitChanged(false)
  }, [quota])

  if (isError) {
    return <p className="text-sm text-destructive">{t('storageQuota.admin.loadError')}</p>
  }

  if (isLoading || !quota) {
    return (
      <Card>
        <CardContent className="space-y-4 p-4">
          <Skeleton className="h-6 w-52" />
          <Skeleton className="h-10 w-full" />
        </CardContent>
      </Card>
    )
  }

  const enteredLimitBytes = parseLimitBytes(limitInput)
  const limitBytes = limitChanged ? enteredLimitBytes : quota.default_limit_bytes
  const limitValid = limitBytes !== null && Number.isSafeInteger(limitBytes) && limitBytes > 0
  const canEdit = quota.applies

  const handleSave = async () => {
    if (limitBytes === null || !Number.isSafeInteger(limitBytes) || limitBytes <= 0) return
    try {
      await updateQuota.mutateAsync({
        enabled,
        default_limit_bytes: limitBytes,
      })
      toast.success(t('storageQuota.admin.saved'))
    } catch {
      toast.error(t('storageQuota.admin.saveError'))
    }
  }

  return (
    <Card>
      <CardContent className="space-y-5 p-4">
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-0.5">
            <Label htmlFor="admin-storage-quota-enabled">{t('storageQuota.admin.enabled')}</Label>
            <p className="text-xs text-muted-foreground">{t('storageQuota.admin.enabledDescription')}</p>
          </div>
          <Switch
            id="admin-storage-quota-enabled"
            checked={enabled}
            onCheckedChange={setEnabled}
            disabled={!canEdit}
            aria-label={t('storageQuota.admin.enabled')}
          />
        </div>

        <div className="grid gap-2 sm:max-w-56">
          <Label htmlFor="admin-storage-quota-limit">{t('storageQuota.admin.limit')}</Label>
          <Input
            id="admin-storage-quota-limit"
            type="number"
            min={1 / GIB}
            step="any"
            inputMode="decimal"
            value={limitInput}
            onChange={(event) => {
              setLimitInput(event.target.value)
              setLimitChanged(true)
            }}
            disabled={!canEdit || !enabled}
            aria-describedby="admin-storage-quota-limit-help"
          />
          <p id="admin-storage-quota-limit-help" className="text-xs text-muted-foreground">
            {t('storageQuota.admin.limitDescription')}
          </p>
          {!limitValid && (
            <p className="text-xs text-destructive">{t('storageQuota.admin.limitError')}</p>
          )}
        </div>

        <dl className="grid gap-2 text-sm sm:grid-cols-2">
          <div>
            <dt className="text-muted-foreground">{t('storageQuota.admin.backend')}</dt>
            <dd className="font-medium">{quota.backend}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">{t('storageQuota.admin.warningThreshold')}</dt>
            <dd className="font-medium">85%</dd>
          </div>
        </dl>

        {!quota.applies && (
          <p className="rounded-md border border-warning/30 bg-warning/10 p-3 text-sm text-warning">
            {t('storageQuota.admin.inactive', { backend: quota.backend })}
          </p>
        )}

        <Button
          type="button"
          onClick={() => void handleSave()}
          disabled={!canEdit || !limitValid || updateQuota.isPending}
        >
          {updateQuota.isPending ? t('storageQuota.admin.saving') : t('storageQuota.admin.save')}
        </Button>
      </CardContent>
    </Card>
  )
}
