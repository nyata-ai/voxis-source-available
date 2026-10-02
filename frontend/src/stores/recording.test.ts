import { describe, it, expect, beforeEach, vi } from 'vitest'
import { act } from '@testing-library/react'

let useRecordingStore: typeof import('./recording').useRecordingStore

describe('useRecordingStore', () => {
  beforeEach(async () => {
    vi.resetModules()
    const mod = await import('./recording')
    useRecordingStore = mod.useRecordingStore
  })

  it('defaults to idle state', () => {
    const state = useRecordingStore.getState()
    expect(state.status).toBe('idle')
    expect(state.captureSource).toBe('microphone')
    expect(state.sessionId).toBeNull()
    expect(state.elapsed).toBe(0)
    expect(state.mimeType).toBeNull()
    expect(state.outboxSize).toBe(0)
    expect(state.lastSavedAt).toBeNull()
    expect(state.errorMessage).toBeNull()
    expect(state.localWriteFailed).toBe(false)
    expect(state.flushFailure).toBeNull()
  })

  it('setLocalWriteFailed records lost local audio', () => {
    act(() => {
      useRecordingStore.getState().setLocalWriteFailed(true)
    })
    expect(useRecordingStore.getState().localWriteFailed).toBe(true)
  })

  it('setFlushFailure records the rejected chunk and clears it again', () => {
    act(() => {
      useRecordingStore.getState().setFlushFailure({ status: 413, seq: 7 })
    })
    expect(useRecordingStore.getState().flushFailure).toEqual({ status: 413, seq: 7 })

    act(() => {
      useRecordingStore.getState().setFlushFailure(null)
    })
    expect(useRecordingStore.getState().flushFailure).toBeNull()
  })

  it('setSessionId updates sessionId', () => {
    act(() => {
      useRecordingStore.getState().setSessionId('sess-123')
    })
    expect(useRecordingStore.getState().sessionId).toBe('sess-123')
  })

  it('setStatus updates status', () => {
    act(() => {
      useRecordingStore.getState().setStatus('recording')
    })
    expect(useRecordingStore.getState().status).toBe('recording')
  })

  it('setElapsed updates elapsed', () => {
    act(() => {
      useRecordingStore.getState().setElapsed(42)
    })
    expect(useRecordingStore.getState().elapsed).toBe(42)
  })

  it('incrementElapsed adds 1 to elapsed', () => {
    act(() => {
      useRecordingStore.getState().setElapsed(10)
    })
    act(() => {
      useRecordingStore.getState().incrementElapsed()
    })
    expect(useRecordingStore.getState().elapsed).toBe(11)
  })

  it('setMimeType updates mimeType', () => {
    act(() => {
      useRecordingStore.getState().setMimeType('audio/webm')
    })
    expect(useRecordingStore.getState().mimeType).toBe('audio/webm')
  })

  it('setCaptureSource updates capture source', () => {
    act(() => {
      useRecordingStore.getState().setCaptureSource('mixed_audio')
    })
    expect(useRecordingStore.getState().captureSource).toBe('mixed_audio')
  })

  it('setOutboxSize updates outboxSize', () => {
    act(() => {
      useRecordingStore.getState().setOutboxSize(5)
    })
    expect(useRecordingStore.getState().outboxSize).toBe(5)
  })

  it('setLastSavedAt updates lastSavedAt', () => {
    const now = Date.now()
    act(() => {
      useRecordingStore.getState().setLastSavedAt(now)
    })
    expect(useRecordingStore.getState().lastSavedAt).toBe(now)
  })

  it('setError sets status to error and errorMessage', () => {
    act(() => {
      useRecordingStore.getState().setError('Something failed')
    })
    expect(useRecordingStore.getState().status).toBe('error')
    expect(useRecordingStore.getState().errorMessage).toBe('Something failed')
  })

  it('reset restores initial state', () => {
    act(() => {
      useRecordingStore.getState().setSessionId('sess-1')
      useRecordingStore.getState().setStatus('recording')
      useRecordingStore.getState().setElapsed(100)
      useRecordingStore.getState().setMimeType('audio/webm')
      useRecordingStore.getState().setOutboxSize(3)
      useRecordingStore.getState().setLocalWriteFailed(true)
      useRecordingStore.getState().setFlushFailure({ status: 409, seq: 2 })
    })

    act(() => {
      useRecordingStore.getState().reset()
    })

    const state = useRecordingStore.getState()
    expect(state.status).toBe('idle')
    expect(state.captureSource).toBe('microphone')
    expect(state.sessionId).toBeNull()
    expect(state.elapsed).toBe(0)
    expect(state.mimeType).toBeNull()
    expect(state.outboxSize).toBe(0)
    expect(state.lastSavedAt).toBeNull()
    expect(state.errorMessage).toBeNull()
    expect(state.localWriteFailed).toBe(false)
    expect(state.flushFailure).toBeNull()
  })
})
