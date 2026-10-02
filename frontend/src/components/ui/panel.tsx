import type { ComponentPropsWithoutRef } from 'react'
import { cn } from '@/lib/utils'

/**
 * The reader's side panel: the bordered, radius-12 card the public Accuracy
 * page uses for its Moments and Download cards, with no shadow (the app defines
 * none). `padded` is the default; a panel whose first row is itself a full-width
 * control (the collapsed audio-integrity row) opts out and pads its rows.
 */
export const PANEL_CLASS = 'flex flex-col rounded-lg border border-border bg-card'

type PanelProps = ComponentPropsWithoutRef<'section'> & {
  padded?: boolean
}

export function Panel({ className, padded = true, ...props }: PanelProps) {
  return <section className={cn(PANEL_CLASS, padded && 'p-5', className)} {...props} />
}

/** The panel's title row: 16/600, 12 px below it before the first hairline. */
export const PANEL_TITLE_CLASS = 'pb-3 text-base font-semibold leading-snug'

type PanelTitleProps = ComponentPropsWithoutRef<'h2'>

export function PanelTitle({ className, ...props }: PanelTitleProps) {
  return <h2 className={cn(PANEL_TITLE_CLASS, className)} {...props} />
}

/** One row inside a panel: a hairline above, at least 44 px tall. */
export const PANEL_ROW_CLASS = 'min-h-11 border-t border-border py-2.5'
