import { useTranslation } from 'react-i18next'
import { useFormatters } from '@/i18n/useFormatters'
import type { DashboardStats } from '@/types/dashboard'
import type { AppFormatters } from '@/i18n/formatters'

type TranslateFn = (key: string, options?: Record<string, unknown>) => string

interface DispatchRowProps {
  stats: DashboardStats
}

// All date arithmetic uses UTC to match the backend's UTC-anchored month
// boundaries in GetDashboardStats. Using local time here causes label/data
// drift in non-UTC timezones (Voxis ships to UTC+7 Jakarta).
function daysInMonthUTC(year: number, monthIndex: number): number {
  return new Date(Date.UTC(year, monthIndex + 1, 0)).getUTCDate()
}

function formatMonthRange(now: Date, formatters: AppFormatters): string {
  return formatters.monthRangeUTC(now)
}

function formatHours(
  seconds: number,
  t: TranslateFn,
  formatters: AppFormatters
): { value: string; unit: string } {
  if (!Number.isFinite(seconds) || seconds <= 0) return { value: '—', unit: '' }
  if (seconds < 3600) {
    return {
      value: formatters.integer(Math.round(seconds / 60)),
      unit: t('dashboard.dispatch.minutes'),
    }
  }
  const hours = seconds / 3600
  return { value: formatters.number(Number(hours.toFixed(1))), unit: t('dashboard.dispatch.hours') }
}

function formatOutput(count: number, t: TranslateFn, formatters: AppFormatters): { value: string; unit: string } {
  if (count === 0) return { value: '—', unit: '' }
  return { value: formatters.integer(count), unit: t('dashboard.dispatch.file', { count }) }
}

function outputLede(
  thisMonth: number,
  lastMonth: number,
  now: Date,
  t: TranslateFn,
  formatters: AppFormatters
): string {
  if (thisMonth === 0) return t('dashboard.dispatch.noCompletions')
  if (lastMonth === 0) return t('dashboard.dispatch.firstThisMonth')
  const diff = thisMonth - lastMonth
  const prevMonthName = formatters.monthNameUTC(
    new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 1, 1))
  )
  if (diff === 0) return t('dashboard.dispatch.evenWith', { month: prevMonthName })
  const sign = diff > 0 ? '+' : '−'
  return t('dashboard.dispatch.vsMonth', {
    sign,
    count: formatters.integer(Math.abs(diff)),
    month: prevMonthName,
  })
}

function dayProgressLede(
  now: Date,
  hasHours: boolean,
  t: TranslateFn,
  formatters: AppFormatters
): string {
  if (!hasHours) return t('dashboard.dispatch.freshMonth')
  const day = now.getUTCDate()
  const total = daysInMonthUTC(now.getUTCFullYear(), now.getUTCMonth())
  return t('dashboard.dispatch.daysInMonth', {
    day: formatters.integer(day),
    total: formatters.integer(total),
    month: formatters.monthNameUTC(now),
  })
}

interface CellProps {
  eyebrow: string
  value: string
  unit: string
  lede: string
  delayMs?: number
}

function DispatchCell({ eyebrow, value, unit, lede, delayMs = 0 }: CellProps) {
  return (
    <div
      className="flex flex-col gap-3 px-6 py-6 animate-[fadeInUp_0.5s_ease-out_both]"
      style={delayMs ? { animationDelay: `${delayMs}ms` } : undefined}
    >
      <p className="text-[0.68rem] font-semibold uppercase tracking-[0.22em] text-muted-foreground/70">
        {eyebrow}
      </p>

      <div className="flex items-baseline gap-2">
        <span className="text-[2.25rem] font-normal leading-none text-foreground sm:text-[2.6rem]">
          {value}
        </span>
        {unit && (
          <span className="text-sm font-medium uppercase tracking-[0.14em] text-muted-foreground/80">
            {unit}
          </span>
        )}
      </div>

      <p className="text-[0.85rem] leading-snug text-muted-foreground/85">
        {lede}
      </p>
    </div>
  )
}

export function DispatchRow({ stats }: DispatchRowProps) {
  const { t } = useTranslation('auth')
  const formatters = useFormatters()
  const now = new Date()
  const hours = formatHours(stats.seconds_this_month, t, formatters)
  const hasHours = stats.seconds_this_month > 0

  const output = formatOutput(stats.completed_this_month, t, formatters)

  return (
    <section
      aria-labelledby="dispatch-heading"
      className="animate-[fadeInUp_0.5s_ease-out_both]"
    >
      <div className="mb-3 flex items-baseline justify-between">
        <h2
          id="dispatch-heading"
          className="text-[0.68rem] font-semibold uppercase tracking-[0.22em] text-muted-foreground/70"
        >
          {t('dashboard.dispatch.thisMonth')}
        </h2>
        <p className="text-[0.85rem] text-muted-foreground/70">
          {formatMonthRange(now, formatters)}
        </p>
      </div>

      <div className="grid grid-cols-1 overflow-hidden rounded-lg border border-border bg-card md:grid-cols-2 md:divide-x md:divide-border/60 divide-y divide-border/60 md:divide-y-0">
        <DispatchCell
          eyebrow={t('dashboard.dispatch.volume')}
          value={hours.value}
          unit={hours.unit}
          lede={dayProgressLede(now, hasHours, t, formatters)}
        />
        <DispatchCell
          eyebrow={t('dashboard.dispatch.output')}
          value={output.value}
          unit={output.unit}
          lede={outputLede(
            stats.completed_this_month,
            stats.completed_last_month,
            now,
            t,
            formatters,
          )}
          delayMs={80}
        />
      </div>
    </section>
  )
}
