import { cn } from '@/lib/utils'
import { speakerInitial, speakerTone } from '@/lib/speaker-tone'

const SIZE_CLASS = {
  /** The chip stack: colour only, no initial. */
  xs: 'h-2.5 w-2.5',
  /** The transcript meta column. */
  sm: 'h-4 w-4 text-caption',
  /** The Speakers panel. */
  md: 'h-5 w-5 text-caption',
} as const

/** The 1.5px ink ring for the second pass through the palette, drawn inset so
 *  a ringed disc is the same size as a plain one. */
const RING_CLASS = 'shadow-[inset_0_0_0_1.5px_var(--color-ink)]'

export interface SpeakerDiscProps {
  /** Zero-based provider speaker number: it picks the colour. */
  speakerIndex: number
  /** The speaker's name, when one is set; the disc shows its first character. */
  name?: string
  size?: keyof typeof SIZE_CLASS
  className?: string
}

/**
 * A speaker's identity disc: their palette colour with their initial in ink.
 * Always decorative (`aria-hidden`): the name next to it is what a screen
 * reader hears.
 */
export function SpeakerDisc({ speakerIndex, name, size = 'sm', className }: SpeakerDiscProps) {
  const tone = speakerTone(speakerIndex)
  return (
    <span
      aria-hidden="true"
      data-testid="speaker-disc"
      className={cn(
        'inline-flex shrink-0 select-none items-center justify-center rounded-full font-semibold leading-none text-ink',
        SIZE_CLASS[size],
        tone.fill,
        tone.ringed && RING_CLASS,
        className,
      )}
    >
      {size === 'xs' ? null : speakerInitial(name, speakerIndex)}
    </span>
  )
}
