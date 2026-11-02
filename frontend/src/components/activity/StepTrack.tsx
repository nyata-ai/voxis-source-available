import { Fragment } from 'react'
import { cn } from '@/lib/utils'

export type StepState = 'done' | 'current' | 'pending' | 'failed'

/** Most steps any job has (Uploading → Encrypting → Scanning → Ready). */
const MAX_STEPS = 4

interface StepTrackProps {
  /** One entry per step the job really has, in order. Steps a deployment
   *  lacks (no malware scan, no briefings) are simply left out by the caller. */
  steps: StepState[]
  /** Names the current step only. It is the accessible label: the dots are
   *  decoration and hidden from assistive technology. */
  label: string
  /** `ink` for the inverted tray card, `paper` everywhere else. */
  surface?: 'paper' | 'ink'
  className?: string
  labelClassName?: string
}

/**
 * A job's progress as a short row of 6px dots joined by a hairline, followed
 * by the name of the step it is on. Quiet Status Rule: no colour per stage —
 * done is solid ink, the current step is an ink ring with the status pulse,
 * steps not reached are faint rings, and only a failed step takes colour.
 */
export function StepTrack({
  steps,
  label,
  surface = 'paper',
  className,
  labelClassName,
}: StepTrackProps) {
  const shown = steps.slice(0, MAX_STEPS)
  return (
    <span className={cn('inline-flex min-w-0 items-center gap-2', className)}>
      <span
        aria-hidden="true"
        data-testid="step-track"
        className={cn(
          'flex shrink-0 items-center',
          surface === 'ink' ? 'text-background' : 'text-foreground',
        )}
      >
        {shown.map((state, index) => (
          <Fragment key={index}>
            {index > 0 && (
              <span
                className={cn(
                  'h-px w-4',
                  state === 'pending' ? 'bg-current opacity-30' : 'bg-current opacity-60',
                )}
              />
            )}
            <StepDot state={state} surface={surface} />
          </Fragment>
        ))}
      </span>
      <span className={cn('min-w-0 truncate', labelClassName)}>{label}</span>
    </span>
  )
}

function StepDot({ state, surface }: { state: StepState; surface: 'paper' | 'ink' }) {
  return (
    <span
      data-step={state}
      className={cn(
        // A finished step fills in over 240ms; reduced motion fills at once.
        // Rings are inset box-shadows, not borders, so a dot keeps its 6 px
        // whichever state it is in.
        'h-1.5 w-1.5 shrink-0 rounded-full transition-colors duration-240 ease-out motion-reduce:transition-none',
        state === 'done' && 'bg-current',
        // A 1.5px ring, so the current step still reads at the pulse's low point.
        state === 'current' &&
          'bg-transparent ring-[1.5px] ring-current ring-inset motion-safe:animate-[status-pulse_1.8s_ease-in-out_infinite]',
        state === 'pending' && 'bg-transparent opacity-40 ring-1 ring-current ring-inset',
        state === 'failed' && (surface === 'ink' ? 'bg-destructive-inverse' : 'bg-destructive'),
      )}
    />
  )
}
