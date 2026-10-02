import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { useApiKeys, useCreateApiKey, useRevokeApiKey } from '@/hooks/useApiKeys'
import { useFormatters } from '@/i18n/useFormatters'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { CreateApiKeyDialog } from './CreateApiKeyDialog'
import { ApiKeyCreatedDialog } from './ApiKeyCreatedDialog'
import type { CreateAPIKeyRequest, CreateAPIKeyResponse } from '@/types/apikey'
import type { AppFormatters } from '@/i18n/formatters'

function formatRelativeTime(dateString: string, t: TFunction, formatters: AppFormatters): string {
  const date = new Date(dateString)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMins = Math.floor(diffMs / 60000)
  const diffHours = Math.floor(diffMs / 3600000)
  const diffDays = Math.floor(diffMs / 86400000)

  if (diffMins < 1) return t('apiKeys.relativeTime.justNow')
  if (diffMins < 60) return t('apiKeys.relativeTime.minutes', { count: diffMins })
  if (diffHours < 24) return t('apiKeys.relativeTime.hours', { count: diffHours })
  if (diffDays < 30) return t('apiKeys.relativeTime.days', { count: diffDays })
  return formatters.date(date)
}

export function ApiKeysSection() {
  const { t } = useTranslation(['settings', 'common'])
  const formatters = useFormatters()
  const { data: keys, isLoading } = useApiKeys()
  const createMutation = useCreateApiKey()
  const revokeMutation = useRevokeApiKey()

  const [showCreateDialog, setShowCreateDialog] = useState(false)
  const [createdResponse, setCreatedResponse] = useState<CreateAPIKeyResponse | null>(null)
  const [revokeKeyId, setRevokeKeyId] = useState<string | null>(null)

  async function handleCreate(req: CreateAPIKeyRequest): Promise<CreateAPIKeyResponse> {
    const response = await createMutation.mutateAsync(req)
    setShowCreateDialog(false)
    setCreatedResponse(response)
    return response
  }

  function handleCloseCreated() {
    setCreatedResponse(null)
  }

  async function handleConfirmRevoke() {
    if (!revokeKeyId) return
    await revokeMutation.mutateAsync(revokeKeyId)
    setRevokeKeyId(null)
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h3 className="text-lg font-medium">{t('apiKeys.title')}</h3>
          <p className="text-sm text-muted-foreground">
            {t('apiKeys.description')}
          </p>
        </div>
        <Button onClick={() => setShowCreateDialog(true)}>
          {t('apiKeys.create')}
        </Button>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      ) : !keys || keys.length === 0 ? (
        <div className="rounded-lg border border-dashed p-8 text-center">
          <p className="text-sm text-muted-foreground">
            {t('apiKeys.empty')}
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {keys.map((key) => (
            <div
              key={key.id}
              className="flex items-center justify-between rounded-lg border p-4"
            >
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium">{key.name}</span>
                  <Badge variant="secondary">{t('apiKeys.scopesCount', { count: key.scopes.length })}</Badge>
                </div>
                <div className="flex items-center gap-4 text-xs text-muted-foreground">
                  <span className="font-mono">{key.key_prefix}...</span>
                  <span>
                    {t('apiKeys.lastUsed', { value: key.last_used_at ? formatRelativeTime(key.last_used_at, t, formatters) : t('apiKeys.never') })}
                  </span>
                  {key.expires_at && (
                    <span>{t('apiKeys.expires', { value: formatters.date(key.expires_at) })}</span>
                  )}
                </div>
              </div>
              <Button
                variant="destructive"
                size="sm"
                onClick={() => setRevokeKeyId(key.id)}
              >
                {t('apiKeys.revoke')}
              </Button>
            </div>
          ))}
        </div>
      )}

      <CreateApiKeyDialog
        open={showCreateDialog}
        onOpenChange={setShowCreateDialog}
        onSubmit={handleCreate}
        isPending={createMutation.isPending}
        error={createMutation.error}
      />

      {createdResponse && (
        <ApiKeyCreatedDialog
          open={!!createdResponse}
          onClose={handleCloseCreated}
          response={createdResponse}
        />
      )}

      <AlertDialog open={!!revokeKeyId} onOpenChange={(open) => !open && setRevokeKeyId(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('apiKeys.revokeDialog.title')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('apiKeys.revokeDialog.description')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common:actions.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={handleConfirmRevoke}>
              {t('apiKeys.revoke')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
