import { describe, it, expect } from 'vitest'
import { ApiError } from '@/lib/api-client'
import { getUploadErrorMessage, isUploadCapacityError } from './upload-utils'

const messages: Record<string, string> = {
  'errors:codes.validation_error': 'Some details need attention.',
  'errors:codes.storage_quota_exceeded': 'Storage quota reached. Free space in Library.',
}

const t = ((key: string) => messages[key] ?? key) as never

describe('getUploadErrorMessage', () => {
  it('uses catalog text when ApiError includes a stable error code', () => {
    const err = new ApiError(400, 'Request failed', {
      error: 'validation_error',
      message: 'server says no',
    })

    expect(getUploadErrorMessage(err, t, 'Upload failed')).toBe('Some details need attention.')
  })

  it('falls back to the localized fallback for ordinary errors', () => {
    const err = new Error('boom')
    expect(getUploadErrorMessage(err, t, 'Upload failed')).toBe('Upload failed')
  })

  it('maps quota rejection while keeping the existing 413 fallback', () => {
    const quotaError = new ApiError(409, 'Conflict', { error: 'storage_quota_exceeded' })
    const tooLarge = new ApiError(413, 'Payload Too Large', { error: 'payload_too_large' })

    expect(getUploadErrorMessage(quotaError, t, 'Upload failed')).toBe('Storage quota reached. Free space in Library.')
    expect(getUploadErrorMessage(tooLarge, t, 'Upload failed')).toBe('Upload failed')
  })

  it('falls back to localized fallback for unknown error', () => {
    expect(getUploadErrorMessage({}, t, 'Upload failed')).toBe('Upload failed')
  })
})

describe('isUploadCapacityError', () => {
  it('waits out a full server: 503 temporarily_unavailable and 429 upload_limit', () => {
    expect(
      isUploadCapacityError(new ApiError(503, 'Busy', { error: 'temporarily_unavailable' }))
    ).toBe(true)
    expect(isUploadCapacityError(new ApiError(429, 'Busy', { error: 'upload_limit' }))).toBe(true)
  })

  it('does not wait on other refusals', () => {
    expect(isUploadCapacityError(new ApiError(429, 'Busy', { error: 'rate_limited' }))).toBe(false)
    expect(isUploadCapacityError(new ApiError(422, 'Long', { error: 'media_too_long' }))).toBe(
      false
    )
    expect(isUploadCapacityError(new Error('network'))).toBe(false)
  })
})
