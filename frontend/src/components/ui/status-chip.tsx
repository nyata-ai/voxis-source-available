import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { statusTone, TONE_CLASS } from '@/lib/status-tone'

interface StatusChipProps {
  status: string
  label?: string
  className?: string
}

/** Converts a snake_case status (e.g. `scan_pending`) to the camelCase key
 * used under the `common.status.*` i18n namespace (e.g. `scanPending`). */
function statusToI18nKey(status: string): string {
  return status.replace(/_([a-z])/g, (_match, letter: string) => letter.toUpperCase())
}

/**
 * Quiet status language, in one place. Live work pulses with the accent;
 * finished work is neutral muted; only real failures/warnings/successes get
 * their own color. Backed by the shared `statusTone` map — never add
 * per-status colors elsewhere, route through this component instead.
 */
export function StatusChip({ status, label, className }: StatusChipProps) {
  const { t } = useTranslation('common')
  const tone = statusTone(status)
  const key = `status.${statusToI18nKey(status)}`
  const translated = t(key, { defaultValue: status })
  const displayLabel = label ?? translated

  return (
    <span
      className={cn(
        'inline-flex h-5 items-center gap-1.5 rounded-full border px-2 text-[0.68rem] font-medium uppercase tracking-wide',
        TONE_CLASS[tone],
        className,
      )}
    >
      {tone === 'live' && (
        <span className="status-pulse h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
      )}
      {displayLabel}
    </span>
  )
}
