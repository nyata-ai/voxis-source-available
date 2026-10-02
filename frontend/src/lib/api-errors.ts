import type { TFunction } from 'i18next'
import { ApiError } from './api-client'

type ApiErrorBody = {
  error?: unknown
  message?: unknown
}

const ERROR_CODE_KEYS = new Set([
  'bad_request',
  'validation_error',
  'invalid_input',
  'not_found',
  'forbidden',
  'unauthorized',
  'conflict',
  'internal_error',
  'service_unavailable',
  'rate_limited',
  'scan_pending',
  'scan_failed',
  'recent_auth_required',
  'state_invalid',
  'storage_quota_exceeded',
  'role_required',
  'upload_limit',
  'media_too_long',
  'model_busy',
])

const STATUS_ERROR_CODES = new Map<number, string>([
  [0, 'network'],
  [401, 'unauthorized'],
  [403, 'forbidden'],
  [404, 'not_found'],
  [408, 'timeout'],
  [409, 'conflict'],
  [429, 'rate_limited'],
  [500, 'internal_error'],
  [503, 'service_unavailable'],
])

function apiErrorBody(error: unknown): ApiErrorBody | null {
  if (!error || typeof error !== 'object' || !('data' in error)) {
    return null
  }

  const data = error.data
  if (!data || typeof data !== 'object') {
    return null
  }
  return data as ApiErrorBody
}

// 4xx statuses a retry can still fix: request timeout, "too early", and rate
// limiting. Every other 4xx is the server rejecting the request itself.
const RETRYABLE_CLIENT_STATUSES = new Set([408, 425, 429])

/**
 * True when retrying the request cannot succeed: the server answered with a
 * 4xx that is not one of the retryable ones (400, 403, 409, 413 …).
 *
 * Network failures (ApiError status 0), timeouts and every 5xx are transient
 * and therefore NOT permanent — callers should keep retrying those.
 */
export function isPermanentHttpError(error: unknown): boolean {
  if (!(error instanceof ApiError)) return false
  if (error.status < 400 || error.status >= 500) return false
  return !RETRYABLE_CLIENT_STATUSES.has(error.status)
}

/** HTTP status of an ApiError, or 0 for anything else (network/unknown). */
export function httpStatusOf(error: unknown): number {
  return error instanceof ApiError ? error.status : 0
}

const DEFAULT_RETRY_ATTEMPTS = 5
const MIN_RETRY_AFTER_SECONDS = 1
const MAX_RETRY_AFTER_SECONDS = 10

/**
 * Retries `fn` when it fails with a 429 (a transient capacity limit, e.g. the
 * recording-lock slot pool), waiting the server's `Retry-After` between
 * attempts. Any other error — including the final 429 once `attempts` is
 * exhausted — is rethrown unchanged. `sleep` is injectable so tests don't
 * need fake timers.
 */
export async function retryOnRateLimit<T>(
  fn: () => Promise<T>,
  opts?: { attempts?: number; sleep?: (ms: number) => Promise<void> }
): Promise<T> {
  const attempts = opts?.attempts ?? DEFAULT_RETRY_ATTEMPTS
  const sleep = opts?.sleep ?? ((ms: number) => new Promise((resolve) => setTimeout(resolve, ms)))

  for (let attempt = 1; ; attempt++) {
    try {
      return await fn()
    } catch (error) {
      if (!(error instanceof ApiError) || error.status !== 429 || attempt >= attempts) {
        throw error
      }
      const seconds = Math.min(
        MAX_RETRY_AFTER_SECONDS,
        Math.max(MIN_RETRY_AFTER_SECONDS, error.retryAfter ?? MIN_RETRY_AFTER_SECONDS)
      )
      await sleep(seconds * 1000)
    }
  }
}

/**
 * Raw backend error code from an ApiError's response body, or null when
 * there isn't one. Most endpoints in this codebase send `{"error": "code"}`
 * (a flat string, handled by getApiErrorMessage above), but some send
 * `{"error": {"code": "...", "message": "..."}}` (an object). This reads
 * either shape so callers that need to branch on a specific code — e.g. a
 * field-level error the generic catalog in errors.json doesn't cover —
 * don't have to duplicate the body-parsing logic.
 */
export function getApiErrorCode(error: unknown): string | null {
  const body = apiErrorBody(error)
  if (!body) return null

  const raw = body.error
  if (typeof raw === 'string') return raw
  if (raw && typeof raw === 'object' && 'code' in raw) {
    const code = (raw as { code?: unknown }).code
    if (typeof code === 'string') return code
  }
  return null
}

export function getApiErrorMessage(error: unknown, t: TFunction, fallback: string): string {
  const body = apiErrorBody(error)

  if (error instanceof ApiError) {
    const bodyCode = typeof body?.error === 'string' ? body.error : null
    const statusCode = STATUS_ERROR_CODES.get(error.status) ?? null
    const code = bodyCode && ERROR_CODE_KEYS.has(bodyCode) ? bodyCode : statusCode

    if (code) {
      return t(`errors:codes.${code}`)
    }
  }

  return fallback
}

// Codes whose catalog message says something a flow's own fallback cannot:
// what to do next, or that the limit is the server's rather than the user's.
const ACTIONABLE_ERROR_CODES = new Set([
  'role_required',
  'upload_limit',
  'media_too_long',
  'model_busy',
])

/**
 * The catalog message for an actionable server error code, or null. For flows
 * (recording start/finish) whose context-specific fallback beats the generic
 * per-status text, but which must still explain these specific refusals.
 */
export function getActionableApiErrorMessage(error: unknown, t: TFunction): string | null {
  if (!(error instanceof ApiError)) return null
  const code = getApiErrorCode(error)
  return code && ACTIONABLE_ERROR_CODES.has(code) ? t(`errors:codes.${code}`) : null
}
