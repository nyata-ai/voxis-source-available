import { ApiError } from '@/lib/api-client'
import { getApiErrorCode, getApiErrorMessage } from '@/lib/api-errors'
import type { TFunction } from 'i18next'
import type { StepState } from '@/components/activity/StepTrack'
import type { UploadItem } from '@/stores/upload'
import type { MediaItem } from '@/types/media'

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

export interface UploadStepView {
  steps: StepState[]
  /** i18n key (media namespace unless it starts with `common:`). */
  labelKey: string
  failed: boolean
}

/**
 * The step track for one upload row: Uploading → Encrypting → Scanning →
 * Ready. "Encrypting" is the server's encrypt-and-store pass, which runs while
 * the browser waits for the upload's answer; "Scanning" is the malware scan
 * the media row reports afterwards. A deployment without a scanner reports
 * `scan_skipped`, and its track simply has no Scanning step. Until the media
 * row has been read (still loading, or a lost connection or deleted file has
 * stopped the polling), the row claims only what it knows: the upload finished.
 * It cannot yet know whether this deployment scans at all.
 */
export function uploadSteps(
  status: Exclude<UploadItem['status'], 'error'>,
  media?: Pick<MediaItem, 'status' | 'scan_status'>,
): UploadStepView {
  switch (status) {
    case 'queued':
      return { steps: ['current', 'pending', 'pending', 'pending'], labelKey: 'upload.step.waiting', failed: false }
    case 'uploading':
      return { steps: ['current', 'pending', 'pending', 'pending'], labelKey: 'upload.step.uploading', failed: false }
    case 'processing':
      return { steps: ['done', 'current', 'pending', 'pending'], labelKey: 'upload.step.encrypting', failed: false }
    case 'complete':
      if (!media) {
        return { steps: ['done', 'done', 'pending', 'pending'], labelKey: 'common:status.completed', failed: false }
      }
      return completedUploadSteps(media)
  }
}

function completedUploadSteps(media: Pick<MediaItem, 'status' | 'scan_status'>): UploadStepView {
  if (media.status === 'failed') {
    return { steps: ['done', 'failed', 'pending', 'pending'], labelKey: 'upload.step.encryptFailed', failed: true }
  }
  switch (media.scan_status) {
    case 'scan_clean':
      return { steps: ['done', 'done', 'done', 'done'], labelKey: 'upload.step.ready', failed: false }
    case 'scan_skipped':
      return { steps: ['done', 'done', 'done'], labelKey: 'upload.step.ready', failed: false }
    case 'scan_infected':
      return { steps: ['done', 'done', 'failed', 'pending'], labelKey: 'common:status.scanInfected', failed: true }
    case 'scan_error':
      return { steps: ['done', 'done', 'failed', 'pending'], labelKey: 'common:status.scanError', failed: true }
    default:
      return { steps: ['done', 'done', 'current', 'pending'], labelKey: 'upload.step.scanning', failed: false }
  }
}
