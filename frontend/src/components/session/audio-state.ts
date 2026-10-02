import { ApiError } from '@/lib/api-client'
import type { MediaItem } from '@/types/media'
import type { SessionAudio } from './session-view'

/**
 * The one place that decides whether a media row's audio can be played,
 * downloaded or re-transcribed. Both the media detail page and the Session
 * Reader's media adapter read it, so a scan verdict cannot mean one thing in
 * one page and something else in the other.
 */

/** Scan verdicts that withhold the bytes. A row with no `scan_status` at all
 *  predates scanning (the field is `omitempty`) and is not treated as blocked. */
const BLOCKING_SCAN_STATUSES = new Set(['scan_pending', 'scan_infected', 'scan_error'])

/** Check if playback is blocked by a scan-related condition. The stream endpoint
 *  is the second source of truth: it answers 409 while a scan is pending and 403
 *  once it has failed, which can arrive before the media row is refetched. */
export function isScanBlocked(scanStatus: string | undefined, streamError: Error | null): boolean {
  if (scanStatus !== undefined && BLOCKING_SCAN_STATUSES.has(scanStatus)) {
    return true
  }
  if (streamError instanceof ApiError && (streamError.status === 409 || streamError.status === 403)) {
    return true
  }
  return false
}

/**
 * Whether the stored file still exists and may be served. Deliberately tolerant
 * of an absent `audio_available` (the backend always emits it; a cached or
 * legacy row that lacks it is treated as present, matching the media detail
 * page's long-standing `!== false` rule).
 */
export function isAudioServable(media: MediaItem | undefined): boolean {
  if (!media) return false
  if (media.audio_deleted_at) return false
  if (media.audio_available === false) return false
  return !isScanBlocked(media.scan_status, null)
}

/**
 * The reader's audio state for a media-backed session, most specific cause
 * first: a deleted file explains itself, a scan verdict outranks a playback
 * failure (the retries were never going to succeed), and only then does an
 * exhausted stream retry count as `failed`.
 */
export function mediaAudioState(
  media: MediaItem | undefined,
  streamError: Error | null,
  streamFailed: boolean,
): SessionAudio['state'] {
  if (!media) return 'unavailable'
  if (media.audio_deleted_at) return 'deleted'
  // One list of blocking verdicts, shared with `isScanBlocked`; the only thing
  // decided here is whether the scan is still running or has already refused.
  if (media.scan_status !== undefined && BLOCKING_SCAN_STATUSES.has(media.scan_status)) {
    return media.scan_status === 'scan_pending' ? 'scan_pending' : 'scan_blocked'
  }
  // The stream endpoint's own verdict, for the window before the media row
  // catches up: 409 means the scan is still running, 403 means it failed.
  if (streamError instanceof ApiError && streamError.status === 409) return 'scan_pending'
  if (streamError instanceof ApiError && streamError.status === 403) return 'scan_blocked'
  if (media.audio_available === false) return 'unavailable'
  if (streamFailed) return 'failed'
  return 'available'
}
