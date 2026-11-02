import { Toaster as SonnerToaster } from 'sonner'

/**
 * While the recording page pins Start, Pause and Stop to a phone's bottom edge,
 * toasts sit above that bar instead of on it. Important because sonner writes its
 * offsets inline; both variables because its own phone layout starts at 600px,
 * narrower than the `sm` breakpoint the bar is pinned below.
 */
const PINNED_BAR_CLEARANCE =
  'max-sm:[body:has([data-pinned-controls])_&]:[--mobile-offset-bottom:calc(5.5rem+env(safe-area-inset-bottom))]! max-sm:[body:has([data-pinned-controls])_&]:[--offset-bottom:calc(5.5rem+env(safe-area-inset-bottom))]!'

export function Toaster() {
  return (
    <SonnerToaster
      position="bottom-right"
      className={PINNED_BAR_CLEARANCE}
      toastOptions={{
        className: 'border border-border bg-background text-foreground',
      }}
    />
  )
}
