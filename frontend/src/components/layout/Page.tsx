import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface PageHeaderProps {
  /** Optional short label above the title (three words or fewer). */
  kicker?: ReactNode
  title: ReactNode
  /** One line under the title. */
  description?: ReactNode
  /** Controls on the right (counts, a refresh button). */
  actions?: ReactNode
  className?: string
}

/**
 * The one app page header: kicker (optional), a `text-page` title at 400, a
 * one-line description, and actions on the right. Every app page uses it so
 * titles stop drifting between six styles.
 */
export function PageHeader({ kicker, title, description, actions, className }: PageHeaderProps) {
  return (
    <header className={cn('flex flex-wrap items-end justify-between gap-x-6 gap-y-3', className)}>
      <div className="min-w-0">
        {kicker && <p className="kicker mb-3">{kicker}</p>}
        <h1 className="text-page font-normal text-foreground">{title}</h1>
        {description && <p className="mt-1 text-body text-muted-foreground">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  )
}

interface PageProps {
  header: ReactNode
  children: ReactNode
  /** `standard` (1024px) for every app page; `wide` (1280px) for the reader. */
  width?: 'standard' | 'wide'
  className?: string
}

/**
 * Page frame: one of the two content widths, the header, 24px to the first
 * section, then sections 32px apart on phones and 40px from md.
 */
export function Page({ header, children, width = 'standard', className }: PageProps) {
  return (
    <div className={cn(width === 'wide' ? 'page-wide' : 'page-standard', className)}>
      {header}
      <div className="page-stack mt-6">{children}</div>
    </div>
  )
}
