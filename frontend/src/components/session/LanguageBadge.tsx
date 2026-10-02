import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'
import { formatLanguageCodes } from '@/lib/languages'

interface LanguageBadgeProps {
  languages: string[]
  className?: string
}

/**
 * Compact "ID·EN" chip for a session's requested languages. Any `auto`
 * member means the request wasn't an explicit language pick, so the badge
 * renders nothing in that case — there's nothing worth surfacing.
 */
export function LanguageBadge({ languages, className }: LanguageBadgeProps) {
  const { t } = useTranslation('common')
  if (languages.length === 0 || languages.includes('auto')) return null

  // formatLanguageCodes does the uppercase mapping and comma-joins it — fine
  // for the aria-label, but the visible chip uses a tighter middle-dot separator.
  const commaCodes = formatLanguageCodes(languages)
  const dotCodes = commaCodes.split(', ').join('·')

  return (
    <span
      className={cn('text-[0.68rem] font-medium uppercase tracking-wide text-muted-foreground', className)}
      aria-label={t('session.languages', { codes: commaCodes })}
    >
      {dotCodes}
    </span>
  )
}
