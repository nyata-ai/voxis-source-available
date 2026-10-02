import { ApiError } from '@/lib/api-client'

/** Resolves a namespaced i18n key to its localized string. */
type TranslateFn = (key: string) => string

/** Whether a rejection is that HTTP status. */
function isStatus(error: unknown, status: number): boolean {
  return error instanceof ApiError && error.status === status
}

/**
 * Whether a fan-out rejection was the rate limiter refusing. Callers use it to
 * decide that the failure is worth automatically retrying later — no other
 * summary failure is.
 */
export function isRateLimited(error: unknown): boolean {
  return isStatus(error, 429)
}

/**
 * The one summary-failure message every reader surface shows — both adapters
 * and the briefing pane. Two statuses say something the user can act on: 503
 * means summarization is not configured at all, and 429 means the request was
 * fine but too soon. Everything else (409, network, 500) falls back to the
 * caller's retry prompt.
 */
export function summaryErrorMessage(error: unknown, t: TranslateFn, fallbackKey: string): string {
  if (isStatus(error, 503)) return t('summary:briefing.aiUnavailable')
  if (isStatus(error, 429)) return t('summary:briefing.rateLimited')
  return t(fallbackKey)
}
