import { ApiError } from '@/lib/api-client'
import { getApiErrorCode, getApiErrorMessage } from '@/lib/api-errors'
import type { TFunction } from 'i18next'

export function getUploadErrorMessage(
  err: unknown,
  t: TFunction,
  fallback: string
): string {
  if (err instanceof ApiError) {
    return getApiErrorMessage(err, t, fallback)
  }

  return fallback
}

/**
 * True for the refusals that mean "the server is full right now": the 503
 * the upload endpoint sends when every upload slot is taken (or scratch disk
 * is short), and the 429 `upload_limit` for too many uploads in progress. Both
 * clear by themselves as uploads finish, so the queue waits instead of
 * failing the file. If waiting runs out, the row shows the server's reason.
 */
export function isUploadCapacityError(err: unknown): err is ApiError {
  if (!(err instanceof ApiError)) return false
  const code = getApiErrorCode(err)
  return (
    (err.status === 503 && code === 'temporarily_unavailable') ||
    (err.status === 429 && code === 'upload_limit')
  )
}

const DEFAULT_RETRY_AFTER_SECONDS = 15
const MAX_RETRY_AFTER_SECONDS = 60

/**
 * Delay before retrying a capacity refusal: the server's Retry-After clamped
 * to 1–60 s, with ±25% jitter so rows refused together do not retry together.
 */
export function uploadRetryDelayMs(err: ApiError, random: () => number = Math.random): number {
  const seconds = err.retryAfter ?? DEFAULT_RETRY_AFTER_SECONDS
  const base = Math.min(Math.max(seconds, 1), MAX_RETRY_AFTER_SECONDS) * 1000
  return Math.round(base * (0.75 + random() * 0.5))
}

/**
 * Bookkeeping for the dropzone's bounded sender, shared by every dropzone
 * instance: the capture dialog unmounts and remounts while uploads keep
 * running, and a remounted instance must not start a row an older one is
 * still sending or has parked while waiting for server capacity.
 */
export const uploadScheduler = {
  started: new Set<string>(),
  inFlight: 0,
}

/** Test hook: forget in-flight bookkeeping between cases. */
export function resetUploadSchedulerForTests(): void {
  uploadScheduler.started.clear()
  uploadScheduler.inFlight = 0
}
