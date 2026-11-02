import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ShieldCheck } from 'lucide-react'
import { cn } from '@/lib/utils'

/**
 * One-line, app-wide note on where data goes in this edition. Copy lives in
 * oss.json `security.*`. An installation stores data wherever its owner runs
 * it, so the strip never claims a storage location.
 *
 * On phones the claims would wrap to several lines on every page, so below
 * `sm` the strip folds: the shield and the first claim stay, and a tap opens
 * the others in place. The wording is never shortened; folding only hides.
 */
export function ConfidentialityStrip() {
  const { t } = useTranslation('oss')
  const [expanded, setExpanded] = useState(false)
  const claims = [t('security.audio'), t('security.transcript'), t('security.boundary')]

  return (
    <div
      role="note"
      aria-label={t('security.label')}
      className="flex items-start gap-x-1.5 border-b px-6 py-1.5 text-caption text-ink-faint"
    >
      <ShieldCheck className="mt-0.5 h-3 w-3 shrink-0" aria-hidden="true" />
      <p className="flex min-w-0 flex-1 flex-wrap items-center gap-x-1.5 gap-y-0.5">
        {claims.map((claim, index) => (
          <span
            key={claim}
            className={cn('items-center gap-1.5', index === 0 || expanded ? 'flex' : 'hidden sm:flex')}
          >
            {index > 0 && <span aria-hidden="true">·</span>}
            <span>{claim}</span>
          </span>
        ))}
      </p>
      <button
        type="button"
        onClick={() => setExpanded((open) => !open)}
        aria-expanded={expanded}
        aria-label={t('security.label')}
        // A 44px tap target that bleeds into the strip's own padding, so the
        // strip itself stays one short line.
        className="-my-2.5 -mr-3 flex h-11 w-11 shrink-0 items-center justify-center rounded-md text-ink-faint hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:hidden"
      >
        <ChevronDown
          className={cn('h-3.5 w-3.5 transition-transform duration-200', expanded && 'rotate-180')}
          aria-hidden="true"
        />
      </button>
    </div>
  )
}
