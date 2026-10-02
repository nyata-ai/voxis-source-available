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
export function HeaderAction({
  icon: Icon,
  label,
  onClick,
  emphasis = 'ghost',
}: HeaderActionProps) {
  const className = cn(
    'inline-flex h-8 items-center gap-1.5 px-2.5 text-xs font-medium uppercase tracking-[0.12em] transition-colors',
    emphasis === 'primary'
      ? 'bg-primary text-primary-foreground hover:bg-primary/90'
      : 'text-foreground/80 hover:bg-accent hover:text-foreground'
  )

  const content = (
    <>
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
      <span className="hidden md:inline">{label}</span>
    </>
  )

  return (
    <button type="button" onClick={onClick} className={className}>
      {content}
    </button>
  )
}
