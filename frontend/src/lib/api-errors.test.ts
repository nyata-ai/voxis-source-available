import { describe, expect, it, vi } from 'vitest'
import { ApiError } from './api-client'
import {
  getActionableApiErrorMessage,
  getApiErrorMessage,
  isPermanentHttpError,
  retryOnRateLimit,
} from './api-errors'

const t = ((key: string) =>
  ({
    'errors:codes.conflict': 'A conflicting operation is already in progress.',
    'errors:codes.storage_quota_exceeded': 'Storage quota reached.',
    'errors:codes.role_required': 'Ask your administrator.',
    'errors:codes.upload_limit': 'Too many uploads.',
    'errors:codes.media_too_long': 'Recording too long.',
    'errors:codes.model_busy': 'Model busy.',
  })[key] ?? key) as never

describe('Voxis-OSS API errors', () => {
  it('uses a localized standard backend error code', () => {
    expect(
      getApiErrorMessage(new ApiError(409, 'Conflict', { error: 'conflict' }), t, 'Fallback')
    ).toBe('A conflicting operation is already in progress.')
  })

  it('keeps storage quota errors readable', () => {
    expect(
      getApiErrorMessage(
        new ApiError(409, 'Conflict', { error: 'storage_quota_exceeded' }),
        t,
        'Fallback'
      )
    ).toBe('Storage quota reached.')
  })

  it('retries rate limits but treats ordinary validation failures as permanent', async () => {
    const retry = vi.fn().mockRejectedValueOnce(new ApiError(429, 'Busy')).mockResolvedValue('ok')
    await expect(retryOnRateLimit(retry, { sleep: async () => {}, attempts: 2 })).resolves.toBe(
      'ok'
    )
    expect(isPermanentHttpError(new ApiError(400, 'Invalid request'))).toBe(true)
  })

  it.each([
    [403, 'role_required', 'Ask your administrator.'],
    [429, 'upload_limit', 'Too many uploads.'],
    [422, 'media_too_long', 'Recording too long.'],
    [429, 'model_busy', 'Model busy.'],
  ])('maps the %i %s envelope to its own message', (status, code, message) => {
    const error = new ApiError(status, 'Refused', { error: code, message: 'server text' })
    expect(getApiErrorMessage(error, t, 'Fallback')).toBe(message)
    expect(getActionableApiErrorMessage(error, t)).toBe(message)
  })

  it('leaves other failures to the calling flow', () => {
    expect(
      getActionableApiErrorMessage(new ApiError(500, 'Boom', { error: 'internal_error' }), t)
    ).toBeNull()
    expect(getActionableApiErrorMessage(new ApiError(409, 'Conflict', null), t)).toBeNull()
    expect(getActionableApiErrorMessage(new Error('network'), t)).toBeNull()
  })
})
