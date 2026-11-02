import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

interface HeaderActionProps {
  icon: LucideIcon
  label: string
  onClick: () => void
  emphasis?: 'primary' | 'ghost'
}

// Every header quick action opens the capture modal, so this is always a
// button. It had a <Link> branch back when Record and Upload navigated.
//
// From md up the sidebar's ink "New transcription" is the shell's one filled
// start button, so the header actions are plain there: Record keeps a hairline
// outline as the first among equals, the others are ghosts. Between sm and md
// (no sidebar on screen) Record is still the ink button.
export function HeaderAction({
  icon: Icon,
  label,
  onClick,
  emphasis = 'ghost',
}: HeaderActionProps) {
  const className = cn(
    'inline-flex h-8 items-center gap-1.5 rounded-md border px-2.5 text-xs font-normal uppercase tracking-[0.12em] transition-colors',
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
    emphasis === 'primary'
      ? 'border-transparent bg-primary text-primary-foreground hover:bg-primary/90 md:border-input md:bg-background md:text-foreground md:hover:bg-accent'
      : 'border-transparent text-foreground/80 hover:bg-accent hover:text-foreground'
  )

  return (
    <button type="button" onClick={onClick} aria-label={label} className={className}>
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
      <span className="hidden md:inline">{label}</span>
    </button>
  )
}
