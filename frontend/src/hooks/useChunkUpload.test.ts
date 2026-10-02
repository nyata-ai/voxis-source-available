import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { ApiError } from '@/lib/api-client'
import { useChunkUpload, ChunkFlushError } from './useChunkUpload'

// Mock chunk-outbox module
vi.mock('@/lib/chunk-outbox', () => ({
  putChunk: vi.fn().mockResolvedValue(1),
  getOldestUnsent: vi.fn().mockResolvedValue(null),
  deleteChunk: vi.fn().mockResolvedValue(undefined),
  countBySession: vi.fn().mockResolvedValue(0),
  deleteBySession: vi.fn().mockResolvedValue(undefined),
}))

// Mock uploadChunk
vi.mock('./useRecordingSession', () => ({
  uploadChunk: vi.fn().mockResolvedValue(undefined),
}))

// Mock the recording store with a stable state object so assertions can look
// at what the hook wrote.
const storeState = {
  setOutboxSize: vi.fn(),
  setLastSavedAt: vi.fn(),
  setFlushFailure: vi.fn(),
  outboxSize: 0,
}
vi.mock('@/stores/recording', () => ({
  useRecordingStore: {
    getState: () => storeState,
  },
}))

describe('useChunkUpload', () => {
  const onBacklogExceeded = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('returns enqueue, waitForDrain, retryFlush and clearSession functions', () => {
    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    expect(typeof result.current.enqueue).toBe('function')
    expect(typeof result.current.waitForDrain).toBe('function')
    expect(typeof result.current.retryFlush).toBe('function')
    expect(typeof result.current.clearSession).toBe('function')
  })

  it('enqueue calls putChunk with correct arguments', async () => {
    const { putChunk } = await import('@/lib/chunk-outbox')

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    const blob = new Blob(['audio'])
    await result.current.enqueue(0, blob)

    expect(putChunk).toHaveBeenCalledWith(
      expect.objectContaining({
        sessionId: 'sess-1',
        seq: 0,
        blob,
      }),
    )
  })

  it('does not enqueue when sessionId is null', async () => {
    const { putChunk } = await import('@/lib/chunk-outbox')

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: null, onBacklogExceeded }),
    )

    const blob = new Blob(['audio'])
    await result.current.enqueue(0, blob)

    expect(putChunk).not.toHaveBeenCalled()
  })

  it('enqueue rejects when the outbox write fails so the caller can react', async () => {
    const { putChunk } = await import('@/lib/chunk-outbox')
    vi.mocked(putChunk).mockRejectedValueOnce(new Error('QuotaExceededError'))

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    await expect(result.current.enqueue(0, new Blob(['audio']))).rejects.toThrow(
      'QuotaExceededError',
    )
  })

  it('enqueue still succeeds when the advisory backlog count fails', async () => {
    const { countBySession } = await import('@/lib/chunk-outbox')
    vi.mocked(countBySession).mockRejectedValue(new Error('idb read failed'))

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    await expect(result.current.enqueue(0, new Blob(['audio']))).resolves.toBeUndefined()
    expect(onBacklogExceeded).not.toHaveBeenCalled()
  })

  it('calls onBacklogExceeded when count exceeds limit', async () => {
    const { countBySession } = await import('@/lib/chunk-outbox')
    vi.mocked(countBySession).mockResolvedValue(11) // > BACKLOG_LIMIT (10)

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    const blob = new Blob(['audio'])
    await result.current.enqueue(0, blob)

    expect(onBacklogExceeded).toHaveBeenCalled()
  })

  it('waitForDrain resolves immediately when sessionId is null', async () => {
    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: null, onBacklogExceeded }),
    )

    // Should resolve without hanging
    await result.current.waitForDrain()
  })

  it('waitForDrain resolves immediately when outbox is empty', async () => {
    const { countBySession } = await import('@/lib/chunk-outbox')
    vi.mocked(countBySession).mockResolvedValue(0)

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    await result.current.waitForDrain()
    // No timeout means it resolved
  })

  it('waitForDrain resolves after the queued chunks upload', async () => {
    const { getOldestUnsent, deleteChunk, countBySession } = await import('@/lib/chunk-outbox')
    vi.mocked(countBySession).mockResolvedValue(1)
    vi.mocked(getOldestUnsent)
      .mockResolvedValueOnce({
        id: 1,
        sessionId: 'sess-1',
        seq: 0,
        blob: new Blob(['audio']),
        createdAt: 1,
      })
      .mockResolvedValue(null)

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    await result.current.waitForDrain()
    expect(deleteChunk).toHaveBeenCalledWith(1)
  })

  it('stops retrying and fails the drain on a permanent 4xx', async () => {
    const { getOldestUnsent, deleteChunk } = await import('@/lib/chunk-outbox')
    const { uploadChunk } = await import('./useRecordingSession')
    vi.mocked(getOldestUnsent).mockResolvedValue({
      id: 7,
      sessionId: 'sess-1',
      seq: 4,
      blob: new Blob(['audio']),
      createdAt: 1,
    })
    vi.mocked(uploadChunk).mockRejectedValue(new ApiError(413, 'Payload Too Large'))

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    await expect(result.current.waitForDrain()).rejects.toBeInstanceOf(ChunkFlushError)

    // One attempt only: retrying a 413 forever is what wedged the UI.
    expect(uploadChunk).toHaveBeenCalledTimes(1)
    expect(deleteChunk).not.toHaveBeenCalled()
    expect(storeState.setFlushFailure).toHaveBeenCalledWith({ status: 413, seq: 4 })
  })

  it('reports the failing status and seq on the drain rejection', async () => {
    const { getOldestUnsent } = await import('@/lib/chunk-outbox')
    const { uploadChunk } = await import('./useRecordingSession')
    vi.mocked(getOldestUnsent).mockResolvedValue({
      id: 9,
      sessionId: 'sess-1',
      seq: 12,
      blob: new Blob(['audio']),
      createdAt: 1,
    })
    vi.mocked(uploadChunk).mockRejectedValue(new ApiError(409, 'Conflict'))

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    const error = await result.current.waitForDrain().catch((err: unknown) => err)
    expect(error).toBeInstanceOf(ChunkFlushError)
    expect((error as ChunkFlushError).status).toBe(409)
    expect((error as ChunkFlushError).seq).toBe(12)

    // A later waitForDrain must fail fast instead of hammering the server.
    await expect(result.current.waitForDrain()).rejects.toBeInstanceOf(ChunkFlushError)
  })

  it('preserves the storage quota error code for the action surface', async () => {
    const { getOldestUnsent } = await import('@/lib/chunk-outbox')
    const { uploadChunk } = await import('./useRecordingSession')
    vi.mocked(getOldestUnsent).mockResolvedValue({
      id: 10,
      sessionId: 'sess-1',
      seq: 13,
      blob: new Blob(['audio']),
      createdAt: 1,
    })
    vi.mocked(uploadChunk).mockRejectedValue(
      new ApiError(409, 'Conflict', { error: 'storage_quota_exceeded' }),
    )

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    const error = await result.current.waitForDrain().catch((err: unknown) => err)
    expect(error).toMatchObject({ status: 409, seq: 13, code: 'storage_quota_exceeded' })
    expect(storeState.setFlushFailure).toHaveBeenCalledWith({
      status: 409,
      seq: 13,
      code: 'storage_quota_exceeded',
    })
  })

  it('preserves the recording-too-long code so the panel can explain it', async () => {
    const { getOldestUnsent } = await import('@/lib/chunk-outbox')
    const { uploadChunk } = await import('./useRecordingSession')
    vi.mocked(getOldestUnsent).mockResolvedValue({
      id: 11,
      sessionId: 'sess-1',
      seq: 240,
      blob: new Blob(['audio']),
      createdAt: 1,
    })
    vi.mocked(uploadChunk).mockRejectedValue(
      new ApiError(422, 'Unprocessable Entity', { error: 'media_too_long', message: 'too long' }),
    )

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    const error = await result.current.waitForDrain().catch((err: unknown) => err)
    expect(error).toMatchObject({ status: 422, seq: 240, code: 'media_too_long' })
  })

  it('keeps retrying transient failures instead of giving up', async () => {
    vi.useFakeTimers()
    const { getOldestUnsent, deleteChunk } = await import('@/lib/chunk-outbox')
    const { uploadChunk } = await import('./useRecordingSession')
    vi.mocked(getOldestUnsent)
      .mockResolvedValueOnce({
        id: 3,
        sessionId: 'sess-1',
        seq: 1,
        blob: new Blob(['audio']),
        createdAt: 1,
      })
      .mockResolvedValue(null)
    vi.mocked(uploadChunk)
      .mockRejectedValueOnce(new ApiError(503, 'Service Unavailable'))
      .mockRejectedValueOnce(new ApiError(0, 'Network error'))
      .mockResolvedValue(undefined)

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    const drain = result.current.waitForDrain()
    await vi.advanceTimersByTimeAsync(4000)
    await drain

    expect(uploadChunk).toHaveBeenCalledTimes(3)
    expect(deleteChunk).toHaveBeenCalledWith(3)
    expect(storeState.setFlushFailure).not.toHaveBeenCalledWith(
      expect.objectContaining({ status: 503 }),
    )
    vi.useRealTimers()
  })

  it('retryFlush clears the recorded failure and drains again', async () => {
    const { getOldestUnsent } = await import('@/lib/chunk-outbox')
    const { uploadChunk } = await import('./useRecordingSession')
    vi.mocked(getOldestUnsent).mockResolvedValueOnce({
      id: 5,
      sessionId: 'sess-1',
      seq: 2,
      blob: new Blob(['audio']),
      createdAt: 1,
    })
    vi.mocked(uploadChunk).mockRejectedValueOnce(new ApiError(400, 'Bad Request'))

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    await expect(result.current.waitForDrain()).rejects.toBeInstanceOf(ChunkFlushError)

    vi.mocked(getOldestUnsent).mockResolvedValue(null)
    await result.current.retryFlush()
    expect(storeState.setFlushFailure).toHaveBeenLastCalledWith(null)
  })

  it('clearSession calls deleteBySession', async () => {
    const { deleteBySession } = await import('@/lib/chunk-outbox')

    const { result } = renderHook(() =>
      useChunkUpload({ sessionId: 'sess-1', onBacklogExceeded }),
    )

    await result.current.clearSession()
    expect(deleteBySession).toHaveBeenCalledWith('sess-1')
    expect(storeState.setFlushFailure).toHaveBeenCalledWith(null)
  })
})
