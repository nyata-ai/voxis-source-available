import { create } from 'zustand'
import type { RecordingCaptureSource, RecordingUIStatus, RecordingMimeType } from '@/types/recording'

/** Server error codes the stuck-upload panel explains in their own words. */
export type RecordingFlushFailureCode = 'storage_quota_exceeded' | 'media_too_long'

export interface RecordingFlushFailure {
  status: number
  seq: number
  code?: RecordingFlushFailureCode
}

interface RecordingState {
  /** Active session ID from backend. */
  sessionId: string | null
  /** UI-facing recording status. */
  status: RecordingUIStatus
  /** Browser capture source for the next recording. */
  captureSource: RecordingCaptureSource
  /** Elapsed seconds (display-only approximation). */
  elapsed: number
  /** Negotiated MIME type. */
  mimeType: RecordingMimeType | null
  /** Number of unsent chunks in IndexedDB outbox. */
  outboxSize: number
  /** Timestamp of last successful server upload. */
  lastSavedAt: number | null
  /** Error message if status is 'error'. */
  errorMessage: string | null
  /**
   * True when a captured chunk could not be persisted to IndexedDB (quota,
   * private mode). Audio for that chunk is gone — the UI must say so instead
   * of completing silently.
   */
  localWriteFailed: boolean
  /**
   * Set when the server permanently rejected a queued chunk (a 4xx a retry
   * cannot fix). Drives the "upload failed" branch of the finalizing screen.
   */
  flushFailure: RecordingFlushFailure | null

  setSessionId: (id: string | null) => void
  setStatus: (status: RecordingUIStatus) => void
  setCaptureSource: (captureSource: RecordingCaptureSource) => void
  setElapsed: (seconds: number) => void
  incrementElapsed: () => void
  setMimeType: (mimeType: RecordingMimeType | null) => void
  setOutboxSize: (size: number) => void
  setLastSavedAt: (timestamp: number | null) => void
  setLocalWriteFailed: (failed: boolean) => void
  setFlushFailure: (failure: RecordingFlushFailure | null) => void
  setError: (message: string) => void
  reset: () => void
}

const initialState = {
  sessionId: null,
  status: 'idle' as RecordingUIStatus,
  captureSource: 'microphone' as RecordingCaptureSource,
  elapsed: 0,
  mimeType: null as RecordingMimeType | null,
  outboxSize: 0,
  lastSavedAt: null as number | null,
  errorMessage: null as string | null,
  localWriteFailed: false,
  flushFailure: null as RecordingFlushFailure | null,
}

export const useRecordingStore = create<RecordingState>((set) => ({
  ...initialState,

  setSessionId: (id) => set({ sessionId: id }),

  setStatus: (status) => set({ status }),

  setCaptureSource: (captureSource) => set({ captureSource }),

  setElapsed: (seconds) => set({ elapsed: seconds }),

  incrementElapsed: () => set((state) => ({ elapsed: state.elapsed + 1 })),

  setMimeType: (mimeType) => set({ mimeType }),

  setOutboxSize: (size) => set({ outboxSize: size }),

  setLastSavedAt: (timestamp) => set({ lastSavedAt: timestamp }),

  setLocalWriteFailed: (failed) => set({ localWriteFailed: failed }),

  setFlushFailure: (failure) => set({ flushFailure: failure }),

  setError: (message) => set({ status: 'error', errorMessage: message }),

  reset: () => set(initialState),
}))
