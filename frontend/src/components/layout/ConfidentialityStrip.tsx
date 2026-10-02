import { ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export function ConfidentialityStrip() {
  const { t } = useTranslation('oss')
  const claims = [t('security.audio'), t('security.transcript'), t('security.boundary')]
  return (
    <div
      role="note"
      aria-label={t('security.label')}
      className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5 border-b px-6 py-1.5 text-[0.68rem] text-muted-foreground"
    >
      <ShieldCheck className="h-3 w-3 shrink-0" aria-hidden="true" />
      {claims.map((claim, index) => (
        <span key={claim} className="flex items-center gap-1.5">
          {index > 0 && <span aria-hidden="true">·</span>}
          <span>{claim}</span>
        </span>
      ))}
    </div>
  )
}
