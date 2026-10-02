import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { AdminRecordingRetentionPanel } from '@/components/admin/AdminRecordingRetentionPanel'
import { AdminStorageQuotaPanel } from '@/components/admin/AdminStorageQuotaPanel'
import {
  useAdminLLMPrompts,
  useResetAdminLLMPrompt,
  useUpdateAdminLLMPrompt,
  type AdminLLMPrompt,
} from '@/hooks/useAdminLLMPrompts'
import { useAdminOpsStats, type AdminOpsEntityStats } from '@/hooks/useAdminOpsStats'
import { useAdminSystemStats } from '@/hooks/useAdminSystemStats'

export function AdminPage() {
  const { t } = useTranslation('oss')
  const system = useAdminSystemStats()
  const operations = useAdminOpsStats()
  const prompts = useAdminLLMPrompts()
  const update = useUpdateAdminLLMPrompt()
  const reset = useResetAdminLLMPrompt()
  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <header>
        <h1 className="text-2xl font-semibold">{t('admin.title')}</h1>
        <p className="text-muted-foreground">{t('admin.description')}</p>
      </header>
      <section className="space-y-3">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-xl font-semibold">{t('admin.system')}</h2>
            <p className="text-sm text-muted-foreground">{t('admin.systemDescription')}</p>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void system.refetch()}
            disabled={system.isFetching}
          >
            {system.isFetching ? t('admin.refreshing') : t('admin.refresh')}
          </Button>
        </div>
        {system.data ? (
          <div className="grid gap-3 md:grid-cols-3">
            <Metric
              title={t('admin.build')}
              value={system.data.build.version || t('admin.unavailable')}
              detail={system.data.build.commit}
            />
            <Metric
              title={t('admin.storage')}
              value={system.data.storage.backend}
              detail={
                system.data.storage.available ? t('admin.available') : system.data.storage.reason
              }
            />
            <Metric
              title={t('admin.dependencies')}
              value={`${system.data.dependencies.filter((item) => item.status === 'up').length}/${system.data.dependencies.length}`}
              detail={t('admin.servicesUp')}
            />
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            {system.isError ? t('admin.systemUnavailable') : t('admin.loadingSystem')}
          </p>
        )}
      </section>
      <section className="space-y-3">
        <h2 className="text-xl font-semibold">{t('admin.pipeline')}</h2>
        {operations.data ? (
          <>
            <div className="grid gap-3 md:grid-cols-3">
              <Entity title={t('admin.media')} stats={operations.data.media} />
              <Entity title={t('admin.scan')} stats={operations.data.scan} />
              <Entity title={t('admin.recordings')} stats={operations.data.recordings} />
              <Entity title={t('admin.transcriptions')} stats={operations.data.transcriptions} />
              <Entity title={t('admin.summaries')} stats={operations.data.summaries} />
              <Metric
                title={t('admin.speechmatics')}
                value={t('admin.completed', {
                  count: operations.data.providers.speechmatics.completed_jobs,
                })}
                detail={t('admin.failed', {
                  count: operations.data.providers.speechmatics.failed_jobs,
                })}
              />
              <Metric
                title={t('admin.gemma')}
                value={t('admin.summariesCompleted', {
                  count: operations.data.providers.gemma.completed_summaries,
                })}
                detail={
                  operations.data.providers.gemma.usage_available
                    ? t('admin.tokensReported', {
                        count: operations.data.providers.gemma.total_tokens,
                      })
                    : t('admin.usageUnavailable')
                }
              />
            </div>
            {operations.data.summary_model && (
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">{t('admin.localModel')}</CardTitle>
                  <CardDescription>{t('admin.modelDescription')}</CardDescription>
                </CardHeader>
                <CardContent className="grid gap-2 text-sm sm:grid-cols-2">
                  <span>
                    {t('admin.model')}: {operations.data.summary_model.model}
                  </span>
                  <span>
                    {t('admin.runtime')}: {operations.data.summary_model.runtime}
                  </span>
                  <span>
                    {t('admin.revision')}: {operations.data.summary_model.revision}
                  </span>
                  <span>
                    {t('admin.quantization')}: {operations.data.summary_model.quantization}
                  </span>
                  <span>
                    {t('admin.usageAvailable')}:{' '}
                    {operations.data.summary_model.usage_available ? t('admin.yes') : t('admin.no')}
                  </span>
                </CardContent>
              </Card>
            )}
          </>
        ) : (
          <p className="text-sm text-muted-foreground">
            {operations.isError ? t('admin.pipelineUnavailable') : t('admin.loadingPipeline')}
          </p>
        )}
      </section>
      <section className="space-y-3">
        <h2 className="text-xl font-semibold">{t('admin.retention')}</h2>
        <AdminRecordingRetentionPanel />
      </section>
      <section className="space-y-3">
        <h2 className="text-xl font-semibold">{t('admin.storageQuota')}</h2>
        <AdminStorageQuotaPanel />
      </section>
      <section className="space-y-3">
        <div>
          <h2 className="text-xl font-semibold">{t('admin.prompts')}</h2>
          <p className="text-sm text-muted-foreground">{t('admin.promptsDescription')}</p>
        </div>
        {prompts.isLoading ? (
          <p className="text-sm text-muted-foreground">{t('admin.loadingPrompts')}</p>
        ) : prompts.isError || !prompts.data ? (
          <p className="text-sm text-destructive">{t('admin.promptsUnavailable')}</p>
        ) : (
          prompts.data.map((prompt) => (
            <PromptCard
              key={prompt.key}
              prompt={prompt}
              saving={update.isPending || reset.isPending}
              onSave={(payload) => update.mutateAsync(payload)}
              onReset={(key) => reset.mutateAsync(key)}
            />
          ))
        )}
      </section>
    </div>
  )
}

function Metric({ title, value, detail }: { title: string; value: string; detail?: string }) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="font-medium">{value}</p>
        {detail && <p className="mt-1 text-sm text-muted-foreground">{detail}</p>}
      </CardContent>
    </Card>
  )
}
function Entity({ title, stats }: { title: string; stats: AdminOpsEntityStats }) {
  const { t } = useTranslation('oss')
  return (
    <Metric
      title={title}
      value={t('admin.total', { count: stats.total })}
      detail={
        Object.entries(stats.by_status)
          .map(([name, count]) => `${name}: ${count}`)
          .join(' · ') || t('admin.noRecords')
      }
    />
  )
}
function PromptCard({
  prompt,
  saving,
  onSave,
  onReset,
}: {
  prompt: AdminLLMPrompt
  saving: boolean
  onSave: (payload: {
    key: string
    system_instruction: string
    user_prompt: string
    model: string
  }) => Promise<unknown>
  onReset: (key: string) => Promise<unknown>
}) {
  const { t } = useTranslation('oss')
  const [system, setSystem] = useState(prompt.system_instruction)
  const [user, setUser] = useState(prompt.user_prompt)
  const [error, setError] = useState('')
  useEffect(() => {
    setSystem(prompt.system_instruction)
    setUser(prompt.user_prompt)
    setError('')
  }, [prompt])
  async function save() {
    setError('')
    try {
      await onSave({
        key: prompt.key,
        system_instruction: system,
        user_prompt: user,
        model: prompt.model,
      })
    } catch {
      setError(t('admin.saveFailed'))
    }
  }
  async function reset() {
    setError('')
    try {
      await onReset(prompt.key)
    } catch {
      setError(t('admin.resetFailed'))
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{prompt.label}</CardTitle>
        <CardDescription>
          {prompt.summary_type} · {prompt.prompt_version}
          {prompt.runtime ? ` · ${prompt.runtime}` : ''}
          {prompt.revision ? ` · ${prompt.revision}` : ''}
          {prompt.quantization ? ` · ${prompt.quantization}` : ''}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault()
            void save()
          }}
        >
          <div className="grid gap-2">
            <Label htmlFor={`${prompt.key}-model`}>{t('admin.servedModel')}</Label>
            <Input id={`${prompt.key}-model`} value={prompt.model} readOnly />
          </div>
          <div className="grid gap-2">
            <Label htmlFor={`${prompt.key}-system`}>{t('admin.presentationGuidance')}</Label>
            <textarea
              id={`${prompt.key}-system`}
              className="min-h-28 w-full rounded-md border bg-background p-3 text-sm"
              value={system}
              onChange={(event) => setSystem(event.target.value)}
              disabled={saving}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor={`${prompt.key}-user`}>{t('admin.taskPrompt')}</Label>
            <textarea
              id={`${prompt.key}-user`}
              className="min-h-28 w-full rounded-md border bg-background p-3 text-sm"
              value={user}
              onChange={(event) => setUser(event.target.value)}
              disabled={saving}
            />
          </div>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => void reset()}
              disabled={saving || !prompt.has_override}
            >
              {t('admin.reset')}
            </Button>
            <Button type="submit" disabled={saving}>
              {t('admin.save')}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
