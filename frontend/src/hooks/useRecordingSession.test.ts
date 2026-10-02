import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'
import {
  useCreateRecordingSession,
  useCompleteRecordingSession,
  usePauseRecordingSession,
  useResumeRecordingSession,
  useRecoverRecordingSession,
  useReleaseRecordingSession,
  useAbandonRecordingSession,
  fetchActiveRecordingSession,
  sendHeartbeat,
  uploadChunk,
  recordingKeys,
} from './useRecordingSession'
import { ApiError, apiClient } from '@/lib/api-client'

// Keep the real ApiError class: the active-session lookup distinguishes a 404
// from a real failure via `instanceof`.
vi.mock('@/lib/api-client', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api-client')>('@/lib/api-client')
  return {
    ...actual,
    apiClient: {
      get: vi.fn(),
      post: vi.fn(),
      patch: vi.fn(),
      delete: vi.fn(),
      upload: vi.fn(),
    },
  }
})

function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
}

function createWrapper(queryClient = createTestQueryClient()) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children)
  }
}

describe('recordingKeys', () => {
  it('generates hierarchical keys', () => {
    expect(recordingKeys.all).toEqual(['recordings'])
    expect(recordingKeys.interrupted()).toEqual(['recordings', 'interrupted'])
    expect(recordingKeys.detail('abc')).toEqual(['recordings', 'detail', 'abc'])
  })
})

describe('useCreateRecordingSession', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.resetAllMocks())

  it('calls POST /recordings with mime_type and microphone_label', async () => {
    const mockSession = {
      id: 'sess-1',
      status: 'recording',
      mime_type: 'audio/webm',
    }
    vi.mocked(apiClient.post).mockResolvedValueOnce(mockSession)

    const { result } = renderHook(() => useCreateRecordingSession(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        mime_type: 'audio/webm',
        microphone_label: 'Built-in Mic',
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(apiClient.post).toHaveBeenCalledWith('/recordings', {
      mime_type: 'audio/webm',
      microphone_label: 'Built-in Mic',
    })
    expect(result.current.data).toEqual(mockSession)
  })
})

describe('useCompleteRecordingSession', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.resetAllMocks())

  it('calls POST /recordings/:id/complete', async () => {
    vi.mocked(apiClient.post).mockResolvedValueOnce({ id: 'sess-1', status: 'completing' })
    const queryClient = createTestQueryClient()
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useCompleteRecordingSession(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate('sess-1')
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(apiClient.post).toHaveBeenCalledWith('/recordings/sess-1/complete')
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['usage', 'stats'] })
  })

  it('retries a 429 (out of lock slots) and succeeds on the next attempt', async () => {
    const rateLimited = new ApiError(429, 'Too Many Requests')
    rateLimited.retryAfter = 0 // keep the test fast; retryOnRateLimit clamps the actual wait
    vi.mocked(apiClient.post)
      .mockRejectedValueOnce(rateLimited)
      .mockResolvedValueOnce({ id: 'sess-1', status: 'completing' })

    const { result } = renderHook(() => useCompleteRecordingSession(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate('sess-1')
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true), { timeout: 5000 })
    expect(apiClient.post).toHaveBeenCalledTimes(2)
  })
})

describe('usePauseRecordingSession', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.resetAllMocks())

  it('calls PATCH /recordings/:id/pause', async () => {
    vi.mocked(apiClient.patch).mockResolvedValueOnce({ id: 'sess-1', status: 'paused' })

    const { result } = renderHook(() => usePauseRecordingSession(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate('sess-1')
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(apiClient.patch).toHaveBeenCalledWith('/recordings/sess-1/pause', {})
  })
})

describe('useResumeRecordingSession', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.resetAllMocks())

  it('calls PATCH /recordings/:id/resume', async () => {
    vi.mocked(apiClient.patch).mockResolvedValueOnce({ id: 'sess-1', status: 'recording' })

    const { result } = renderHook(() => useResumeRecordingSession(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate('sess-1')
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(apiClient.patch).toHaveBeenCalledWith('/recordings/sess-1/resume', {})
  })
})

describe('useRecoverRecordingSession', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.resetAllMocks())

  it('calls POST /recordings/:id/recover', async () => {
    vi.mocked(apiClient.post).mockResolvedValueOnce({ id: 'sess-1', status: 'completing' })

    const { result } = renderHook(() => useRecoverRecordingSession(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate('sess-1')
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(apiClient.post).toHaveBeenCalledWith('/recordings/sess-1/recover')
  })
})

describe('useReleaseRecordingSession', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.resetAllMocks())

  it('calls POST /recordings/:id/release', async () => {
    vi.mocked(apiClient.post).mockResolvedValueOnce({ id: 'sess-1', status: 'interrupted' })

    const { result } = renderHook(() => useReleaseRecordingSession(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate('sess-1')
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(apiClient.post).toHaveBeenCalledWith('/recordings/sess-1/release')
  })
})

describe('fetchActiveRecordingSession', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.resetAllMocks())

  it('returns the active session', async () => {
    const session = { id: 'sess-active', status: 'recording', chunk_count: 3 }
    vi.mocked(apiClient.get).mockResolvedValueOnce(session)

    await expect(fetchActiveRecordingSession()).resolves.toEqual(session)
    expect(apiClient.get).toHaveBeenCalledWith('/recordings/active')
  })

  it('returns null when the server reports no active session', async () => {
    vi.mocked(apiClient.get).mockRejectedValueOnce(new ApiError(404, 'Not Found'))

    await expect(fetchActiveRecordingSession()).resolves.toBeNull()
  })

  it('propagates real failures instead of pretending there is no session', async () => {
    vi.mocked(apiClient.get).mockRejectedValueOnce(new ApiError(500, 'Internal Server Error'))

    await expect(fetchActiveRecordingSession()).rejects.toBeInstanceOf(ApiError)
  })
})

describe('useAbandonRecordingSession', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.resetAllMocks())

  it('calls DELETE /recordings/:id', async () => {
    vi.mocked(apiClient.delete).mockResolvedValueOnce(null)
    const queryClient = createTestQueryClient()
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useAbandonRecordingSession(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate('sess-1')
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(apiClient.delete).toHaveBeenCalledWith('/recordings/sess-1')
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['usage', 'stats'] })
  })
})

describe('uploadChunk', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.resetAllMocks())

  it('uploads chunk as multipart form with file and seq', async () => {
    vi.mocked(apiClient.upload).mockResolvedValueOnce(undefined)

    const blob = new Blob(['audio-data'], { type: 'audio/webm' })
    await uploadChunk('sess-1', 3, blob)

    expect(apiClient.upload).toHaveBeenCalledTimes(1)
    const [endpoint, formData] = vi.mocked(apiClient.upload).mock.calls[0]
    expect(endpoint).toBe('/recordings/sess-1/chunks')
    expect(formData).toBeInstanceOf(FormData)
    expect(formData.get('seq')).toBe('3')
    expect(formData.get('file')).toBeInstanceOf(Blob)
  })
})

describe('sendHeartbeat', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.resetAllMocks())

  it('calls POST /recordings/:id/heartbeat', async () => {
    vi.mocked(apiClient.post).mockResolvedValueOnce({ status: 'ok' })

    await sendHeartbeat('sess-1')

    expect(apiClient.post).toHaveBeenCalledWith('/recordings/sess-1/heartbeat')
  })
})
