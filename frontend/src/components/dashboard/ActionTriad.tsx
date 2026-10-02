import { Mic, Upload } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useCaptureStore } from '@/stores/capture'

export function ActionTriad() {
  const { t } = useTranslation('oss')
  const openCapture = useCaptureStore((state) => state.openCapture)
  return (
    <section aria-labelledby="oss-actions-heading">
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
        <h2 id="oss-actions-heading" className="text-sm font-semibold">
          {t('dashboard.title')}
        </h2>
        <p className="text-sm text-muted-foreground">{t('dashboard.description')}</p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <Action
          label={t('dashboard.record')}
          description={t('dashboard.recordBody')}
          icon={<Mic className="h-5 w-5" />}
          onClick={() => openCapture('record')}
        />
        <Action
          label={t('dashboard.upload')}
          description={t('dashboard.uploadBody')}
          icon={<Upload className="h-5 w-5" />}
          onClick={() => openCapture('upload')}
        />
      </div>
    </section>
  )
}

function Action({
  label,
  description,
  icon,
  onClick,
}: {
  label: string
  description: string
  icon: ReactNode
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="rounded-lg border bg-card p-5 text-left transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <span className="mb-4 flex h-10 w-10 items-center justify-center rounded-full bg-primary/10 text-primary">
        {icon}
      </span>
      <span className="block text-lg font-medium">{label}</span>
      <span className="mt-1 block text-sm text-muted-foreground">{description}</span>
    </button>
  )
}
