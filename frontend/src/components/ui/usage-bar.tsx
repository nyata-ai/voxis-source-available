import { cn } from '@/lib/utils'

interface UsageBarProps {
  used: number
  total: number
  /** Overrides the default host-usage thresholds for a policy-backed meter. */
  state?: 'ok' | 'warning' | 'full'
  className?: string
}

// UsageBar renders a thin horizontal usage indicator; turns warning >80%, destructive >90%.
export function UsageBar({ used, total, state, className }: UsageBarProps) {
  const pct = total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0
  const tone = state
    ? state === 'full'
      ? 'bg-destructive'
      : state === 'warning'
        ? 'bg-warning'
        : 'bg-primary'
    : pct > 90
      ? 'bg-destructive'
      : pct > 80
        ? 'bg-warning'
        : 'bg-primary'
  return (
    <div className={cn('h-2 w-full overflow-hidden rounded-full bg-muted', className)}
      role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}>
      <div className={cn('h-full rounded-full transition-all', tone)} style={{ width: `${pct}%` }} />
    </div>
  )
}
