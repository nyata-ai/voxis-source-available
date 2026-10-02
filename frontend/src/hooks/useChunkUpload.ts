import { useEffect, useRef } from 'react'
import {
  putChunk,
  getOldestUnsent,
  deleteChunk,
  countBySession,
  deleteBySession,
} from '@/lib/chunk-outbox'
import { httpStatusOf, isPermanentHttpError } from '@/lib/api-errors'
import { ApiError } from '@/lib/api-client'
import { uploadChunk } from './useRecordingSession'
import { useRecordingStore, type RecordingFlushFailureCode } from '@/stores/recording'

/** Maximum unsent chunks before auto-pause. */
const BACKLOG_LIMIT = 10

/** Retry delays: 1s, 2s, 4s, 8s, capped at 15s. */
const RETRY_DELAYS = [1000, 2000, 4000, 8000, 15000]

/** Pause duration after max retries exhausted (30 seconds). */
const RETRY_PAUSE_MS = 30_000

/**
 * Raised when the server permanently rejected a queued chunk. Distinguishes a
 * hopeless flush from a slow one so `waitForDrain()` callers can offer the user
 * a way out instead of spinning forever.
 */
export class ChunkFlushError extends Error {
  constructor(
    public status: number,
    public seq: number,
    public code?: RecordingFlushFailureCode,
  ) {
    super(`chunk ${seq} rejected with status ${status}`)
    this.name = 'ChunkFlushError'
  }
}

type UploadOutcome = 'uploaded' | 'permanent' | 'exhausted'

interface UseChunkUploadOptions {
  sessionId: string | null
  /** Called when backlog exceeds limit. Parent should pause recording. */
  onBacklogExceeded: () => void
}

interface UseChunkUploadReturn {
  /**
   * Enqueue a blob for upload. Rejects when the blob could not be persisted —
   * the caller MUST treat a rejection as lost audio.
   */
  enqueue: (seq: number, blob: Blob) => Promise<void>
  /**
   * Wait for all pending chunks to drain. Resolves when the outbox is empty,
   * rejects with `ChunkFlushError` when a chunk was permanently rejected.
   */
  waitForDrain: () => Promise<void>
  /** Clear a recorded permanent failure and drain again. */
  retryFlush: () => Promise<void>
  /** Clear all entries for the session. */
  clearSession: () => Promise<void>
}

/**
 * Manages the IndexedDB outbox flush loop.
 *
 * Chunks are enqueued via `enqueue()`, then a background loop
 * reads the oldest unsent entry, uploads it, deletes it, and repeats.
 * Only one upload in-flight at a time (serial, in-order).
 */
export function useChunkUpload({
  sessionId,
  onBacklogExceeded,
}: UseChunkUploadOptions): UseChunkUploadReturn {
  const flushingRef = useRef(false)
  const drainWaitersRef = useRef<
    Array<{ resolve: () => void; reject: (error: Error) => void }>
  >([])
  const permanentFailureRef = useRef<{
    status: number
    seq: number
    code?: RecordingFlushFailureCode
  } | null>(null)
  const sessionIdRef = useRef(sessionId)
  sessionIdRef.current = sessionId

  // Ref for the callback to avoid stale closures
  const onBacklogExceededRef = useRef(onBacklogExceeded)
  onBacklogExceededRef.current = onBacklogExceeded

  // Update outbox size periodically
  const updateOutboxSize = async () => {
    if (!sessionIdRef.current) return
    try {
      const count = await countBySession(sessionIdRef.current)
      useRecordingStore.getState().setOutboxSize(count)
    } catch {
      // IndexedDB may fail in rare cases; ignore
    }
  }

  /**
   * Hand the flush outcome to every `waitForDrain()` caller. Called on every
   * exit path of the loop so a waiter can never miss the wake-up.
   */
  function settleDrainWaiters() {
    const waiters = drainWaitersRef.current
    drainWaitersRef.current = []
    const failure = permanentFailureRef.current
    for (const waiter of waiters) {
      if (failure) {
        waiter.reject(new ChunkFlushError(failure.status, failure.seq, failure.code))
      } else {
        waiter.resolve()
      }
    }
  }

  function markPermanentFailure(status: number, seq: number, code?: RecordingFlushFailureCode) {
    const failure = code ? { status, seq, code } : { status, seq }
    permanentFailureRef.current = failure
    useRecordingStore.getState().setFlushFailure(failure)
  }

  /** Upload one entry, retrying only errors a retry can fix. */
  async function uploadWithRetry(seq: number, blob: Blob): Promise<UploadOutcome> {
    for (let attempt = 0; attempt < RETRY_DELAYS.length; attempt++) {
      const currentSessionId = sessionIdRef.current
      if (!currentSessionId) return 'exhausted'
      try {
        await uploadChunk(currentSessionId, seq, blob)
        return 'uploaded'
      } catch (err) {
        // A 4xx the server will keep rejecting (gap in the sequence, chunk too
        // large, session closed) must break the loop: retrying it forever
        // wedges the "Finalizing…" screen with no way out.
        if (isPermanentHttpError(err)) {
          markPermanentFailure(httpStatusOf(err), seq, flushFailureCode(err))
          return 'permanent'
        }
        await sleep(RETRY_DELAYS[attempt])
      }
    }
    return 'exhausted'
  }

  async function flushLoop() {
    if (flushingRef.current) return
    // A recorded permanent failure needs an explicit retryFlush(); otherwise
    // every enqueue would re-run into the same rejection.
    if (permanentFailureRef.current) {
      settleDrainWaiters()
      return
    }
    flushingRef.current = true

    try {
      while (sessionIdRef.current && !permanentFailureRef.current) {
        const entry = await getOldestUnsent(sessionIdRef.current)
        if (!entry || entry.id === undefined) break

        const outcome = await uploadWithRetry(entry.seq, entry.blob)
        if (outcome === 'permanent') break

        if (outcome === 'uploaded') {
          await deleteChunk(entry.id)
          useRecordingStore.getState().setLastSavedAt(Date.now())
          await updateOutboxSize()
        } else {
          // All retries exhausted — pause before trying again
          await updateOutboxSize()
          await sleep(RETRY_PAUSE_MS)
        }
      }
    } catch (err) {
      // Reading or deleting from IndexedDB failed. Surface it as a flush
      // failure rather than leaving drain waiters hanging forever.
      markPermanentFailure(httpStatusOf(err), -1)
    } finally {
      flushingRef.current = false
      settleDrainWaiters()
    }
  }

  /** Fire-and-forget flush kick that can never raise an unhandled rejection. */
  function startFlush() {
    void flushLoop().catch(() => {
      // flushLoop handles its own errors; this is a belt-and-braces guard.
    })
  }

  // Start flush loop when sessionId changes
  useEffect(() => {
    if (sessionId) {
      startFlush()
    }
  }, [sessionId]) // eslint-disable-line react-hooks/exhaustive-deps

  async function enqueue(seq: number, blob: Blob) {
    if (!sessionIdRef.current) return

    // Deliberately unguarded: a failed write means the audio is gone, and the
    // caller must react (stop the recording, tell the user).
    await putChunk({
      sessionId: sessionIdRef.current,
      seq,
      blob,
      createdAt: Date.now(),
    })

    await updateOutboxSize()

    // Backlog check is advisory — never fail an otherwise successful write.
    try {
      const count = await countBySession(sessionIdRef.current)
      if (count > BACKLOG_LIMIT) {
        onBacklogExceededRef.current()
      }
    } catch {
      // Ignore: the write itself succeeded.
    }

    // Kick the flush loop if not running
    startFlush()
  }

  function waitForDrain(): Promise<void> {
    const failure = permanentFailureRef.current
    if (failure) {
      return Promise.reject(new ChunkFlushError(failure.status, failure.seq, failure.code))
    }
    // If there's nothing to drain, resolve immediately
    if (!sessionIdRef.current) {
      return Promise.resolve()
    }

    // Register before kicking the loop: every loop exit settles the waiter
    // list, so registration can never race a wake-up.
    return new Promise<void>((resolve, reject) => {
      drainWaitersRef.current.push({ resolve, reject })
      startFlush()
    })
  }

  function retryFlush(): Promise<void> {
    permanentFailureRef.current = null
    useRecordingStore.getState().setFlushFailure(null)
    return waitForDrain()
  }

  async function clearSession() {
    if (!sessionIdRef.current) return
    permanentFailureRef.current = null
    useRecordingStore.getState().setFlushFailure(null)
    await deleteBySession(sessionIdRef.current)
    await updateOutboxSize()
  }

  return { enqueue, waitForDrain, retryFlush, clearSession }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function flushFailureCode(error: unknown): RecordingFlushFailureCode | undefined {
  if (!(error instanceof ApiError) || !error.data || typeof error.data !== 'object') return undefined
  const code = 'error' in error.data ? error.data.error : undefined
  return code === 'storage_quota_exceeded' || code === 'media_too_long' ? code : undefined
}
