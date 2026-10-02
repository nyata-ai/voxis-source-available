import type { ComponentPropsWithoutRef } from 'react'
import { cn } from '@/lib/utils'

/**
 * The chip vocabulary the app shares with the public Accuracy pages: 12/600 on
 * paper-soft, a hairline ring, fully rounded, muted ink. Every chip, pill and
 * pill-shaped control in the reader composes this one class so a token change
 * lands everywhere at once. Dark mode falls back to the app's muted surface.
 */
export const CHIP_CLASS =
  'inline-flex items-center gap-1.5 rounded-full border border-secondary bg-paper-soft ' +
  'px-2.5 py-[3px] text-xs font-semibold tracking-[0.02em] text-ink-muted ' +
  'dark:border-border dark:bg-muted dark:text-muted-foreground'

/** The same recipe for interactive pills (buttons, select triggers): hover and
 *  focus states on top of `CHIP_CLASS`, no vertical padding so the caller sets
 *  the height (44 px on phones, 32 px from `sm`). */
export const PILL_CLASS =
  'inline-flex items-center rounded-full border border-secondary bg-paper-soft ' +
  'font-semibold text-ink-muted transition-colors hover:border-coral hover:text-coral ' +
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 ' +
  'dark:border-border dark:bg-muted dark:text-muted-foreground'

type ChipProps = ComponentPropsWithoutRef<'span'>

/** A static, non-interactive chip. For anything clickable use `PILL_CLASS`
 *  on a `<button>` so the element keeps its native semantics. */
export function Chip({ className, ...props }: ChipProps) {
  return <span className={cn(CHIP_CLASS, className)} {...props} />
}
