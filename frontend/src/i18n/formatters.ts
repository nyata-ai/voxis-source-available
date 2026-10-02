import { DEFAULT_LOCALE } from './config'

type DateInput = Date | string | number | null | undefined

function toValidDate(value: DateInput): Date | null {
  if (value === null || value === undefined) return null
  const date = value instanceof Date ? value : new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

function dayDiffUTC(date: Date, now: Date): number {
  const dateDay = Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate())
  const nowDay = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate())
  return Math.round((dateDay - nowDay) / 86_400_000)
}

export function createFormatters(locale: string = DEFAULT_LOCALE) {
  const resolvedLocale = locale || DEFAULT_LOCALE

  const integerFormatter = new Intl.NumberFormat(resolvedLocale, {
    maximumFractionDigits: 0,
  })
  const numberFormatter = new Intl.NumberFormat(resolvedLocale)
  const compactFormatter = new Intl.NumberFormat(resolvedLocale, {
    notation: 'compact',
    maximumFractionDigits: 1,
  })
  const dateFormatter = new Intl.DateTimeFormat(resolvedLocale, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
  const dateTimeFormatter = new Intl.DateTimeFormat(resolvedLocale, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
  const longDateFormatter = new Intl.DateTimeFormat(resolvedLocale, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
    timeZone: 'UTC',
  })
  const asOfDateFormatter = new Intl.DateTimeFormat(resolvedLocale, {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    timeZone: 'UTC',
  })
  const monthUTCFormatter = new Intl.DateTimeFormat(resolvedLocale, {
    month: 'long',
    timeZone: 'UTC',
  })
  // Credit expiry is a calendar date meaningful to Indonesia-based users
  // (Voxis's target market), so it renders in Asia/Jakarta (WIB, UTC+7, no
  // DST) regardless of the viewer's device timezone. Rendering the underlying
  // TIMESTAMPTZ in the browser's local zone would show the wrong calendar day
  // for anyone not also in WIB.
  const dateJakartaFormatter = new Intl.DateTimeFormat(resolvedLocale, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    timeZone: 'Asia/Jakarta',
  })
  const relativeTimeFormatter = new Intl.RelativeTimeFormat(resolvedLocale, {
    numeric: 'auto',
  })

  return {
    date(value: DateInput): string {
      const date = toValidDate(value)
      return date ? dateFormatter.format(date) : ''
    },
    dateTime(value: DateInput): string {
      const date = toValidDate(value)
      return date ? dateTimeFormatter.format(date) : ''
    },
    longDate(value: DateInput): string {
      const date = toValidDate(value)
      return date ? longDateFormatter.format(date) : ''
    },
    asOfDate(value: DateInput): string {
      const date = toValidDate(value)
      return date ? asOfDateFormatter.format(date).toLocaleUpperCase(resolvedLocale) : ''
    },
    relativeDate(value: DateInput, now: Date = new Date()): string {
      const date = toValidDate(value)
      if (!date) return ''
      const diffDays = dayDiffUTC(date, now)
      if (Math.abs(diffDays) <= 6) {
        return relativeTimeFormatter.format(diffDays, 'day')
      }
      return dateFormatter.format(date)
    },
    number(value: number): string {
      return numberFormatter.format(value)
    },
    integer(value: number): string {
      return integerFormatter.format(value)
    },
    compactNumber(value: number): string {
      return compactFormatter.format(value)
    },
    dateJakarta(value: DateInput): string {
      const date = toValidDate(value)
      return date ? dateJakartaFormatter.format(date) : ''
    },
    monthNameUTC(value: DateInput): string {
      const date = toValidDate(value)
      return date ? monthUTCFormatter.format(date) : ''
    },
    monthRangeUTC(value: DateInput): string {
      const date = toValidDate(value)
      if (!date) return ''
      return `${monthUTCFormatter.format(date)} 1–${date.getUTCDate()}`
    },
  }
}

export type AppFormatters = ReturnType<typeof createFormatters>
