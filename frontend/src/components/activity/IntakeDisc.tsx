import { Link2, Mic, Upload } from 'lucide-react'
import { INTAKE_DISC_CLASS } from '@/lib/intake-tone'
import { cn } from '@/lib/utils'
import type { Intake } from './activity-labels'

const ICON: Record<Intake, typeof Mic> = { record: Mic, upload: Upload, link: Link2 }

/**
 * The intake colour (peach = record, mint = upload, cream = link) the Desk
 * tiles already use, carried onto rows that stand for one piece of work.
 * Decorative: the row's own text says what the work is.
 */
export function IntakeDisc({ intake, className }: { intake: Intake; className?: string }) {
  const Icon = ICON[intake]
  return (
    <span
      aria-hidden="true"
      data-intake={intake}
      className={cn(
        'inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-full',
        INTAKE_DISC_CLASS[intake],
        className
      )}
    >
      <Icon className="h-3.5 w-3.5" strokeWidth={2} />
    </span>
  )
}
