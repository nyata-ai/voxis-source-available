import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider, focusManager } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'
import { useMediaDetail, useMediaStreamUrl, useUploadMedia, useUpdateMedia, useDeleteMedia, mediaKeys, mediaPollInterval } from './useMedia'
import { ApiError, apiClient, resolveApiUrl } from '@/lib/api-client'
import type { MediaItem } from '@/types/media'

vi.mock('@/lib/api-client', () => ({
  ApiError: class ApiError extends Error {
    status: number
    data?: unknown
    constructor(status: number, message: string, data?: unknown) {
      super(message)
      this.status = status
      this.data = data
    }
  },
  apiClient: {
    get: vi.fn(),
    getStreamUrl: vi.fn(),
    upload: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
  },
  resolveApiUrl: vi.fn((path: string) => `https://api.example.test${path}`),
}))

function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
      mutations: {
        retry: false,
      },
    },
  })
}

function createWrapper(queryClient = createTestQueryClient()) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children)
  }
}

describe('mediaKeys', () => {
  it('should generate hierarchical keys', () => {
    expect(mediaKeys.all).toEqual(['media'])
    expect(mediaKeys.detail('abc-123')).toEqual(['media', 'detail', 'abc-123'])
  })
})

describe('useMediaDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should fetch media detail by id', async () => {
    const mockMedia = {
      id: 'media-1',
      filename: 'recording.mp3',
      content_type: 'audio/mpeg',
      size: 1024000,
      duration: 120,
      status: 'ready',
      created_at: '2026-01-15T10:00:00Z',
    }

    vi.mocked(apiClient.get).mockResolvedValueOnce(mockMedia)

    const { result } = renderHook(() => useMediaDetail('media-1'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data).toEqual(mockMedia)
    expect(apiClient.get).toHaveBeenCalledWith('/media/media-1')
  })

  it('should not fetch when id is empty', () => {
    const { result } = renderHook(() => useMediaDetail(''), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(apiClient.get).not.toHaveBeenCalled()
  })

})

// The poll decision is a pure function so it can be asserted directly: driving
// it through React Query would cost seconds of real time per case for one
// boolean.
describe('mediaPollInterval', () => {
  function row(overrides: Partial<MediaItem>): MediaItem {
    return {
      id: 'media-1',
      filename: 'recording.mp3',
      content_type: 'audio/mpeg',
      size: 1024,
      status: 'ready',
      created_at: '2026-01-15T10:00:00Z',
      ...overrides,
    } as MediaItem
  }

  it('polls while the file is still being stored', () => {
    expect(mediaPollInterval(row({ status: 'pending' }))).toBeGreaterThan(0)
    expect(mediaPollInterval(row({ status: 'encrypting' }))).toBeGreaterThan(0)
  })

  // A row reaches `ready` while the malware scan is still running, and the scan
  // verdict is what decides whether playback, download and transcription may be
  // offered. Stopping at `ready` froze a freshly uploaded file in `scan_pending`
  // until the user reloaded the page by hand.
  it('keeps polling a ready row whose scan has not landed', () => {
    expect(mediaPollInterval(row({ status: 'ready', scan_status: 'scan_pending' }))).toBeGreaterThan(
      0,
    )
  })

  it('stops once every verdict is in', () => {
    expect(mediaPollInterval(row({ status: 'ready', scan_status: 'scan_clean' }))).toBe(false)
    expect(mediaPollInterval(row({ status: 'ready', scan_status: 'scan_infected' }))).toBe(false)
    expect(mediaPollInterval(row({ status: 'ready', scan_status: 'scan_skipped' }))).toBe(false)
    expect(mediaPollInterval(row({ status: 'failed' }))).toBe(false)
    // A row that predates scanning carries no `scan_status` at all.
    expect(mediaPollInterval(row({ status: 'ready' }))).toBe(false)
  })

  it('has nothing to poll for before the first read lands', () => {
    expect(mediaPollInterval(undefined)).toBe(false)
  })
})

describe('useUploadMedia', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should call upload endpoint with FormData', async () => {
    const mockMedia = {
      id: 'media-new',
      filename: 'upload.mp3',
      content_type: 'audio/mpeg',
      size: 2048000,
      duration: 0,
      status: 'processing',
      created_at: '2026-01-15T10:00:00Z',
    }

    vi.mocked(apiClient.upload).mockResolvedValueOnce(mockMedia)

    const queryClient = createTestQueryClient()
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderHook(() => useUploadMedia(), {
      wrapper: createWrapper(queryClient),
    })

    const formData = new FormData()
    formData.append('file', new File(['audio'], 'upload.mp3', { type: 'audio/mpeg' }))

    const onProgress = vi.fn()

    await act(async () => {
      result.current.mutate({ formData, onProgress })
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.upload).toHaveBeenCalledWith('/media/upload', formData, onProgress)
    expect(result.current.data).toEqual(mockMedia)
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['usage', 'stats'] })
  })
})

describe('useMediaStreamUrl', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should fetch stream URL and resolve to absolute API URL', async () => {
    vi.mocked(apiClient.getStreamUrl).mockResolvedValueOnce({
      url: '/api/v1/media/media-1/stream?token=abc',
      expires_at: '2026-03-01T00:00:00Z',
    })

    const { result } = renderHook(() => useMediaStreamUrl('media-1'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.getStreamUrl).toHaveBeenCalledWith('media-1')
    expect(resolveApiUrl).toHaveBeenCalledWith('/api/v1/media/media-1/stream?token=abc')
    expect(result.current.data?.url).toBe('https://api.example.test/api/v1/media/media-1/stream?token=abc')
  })

  // `staleTime: 0` plus the client's global refetchOnWindowFocus would mint a
  // NEW signed URL on every alt-tab, and a changed `src` resets the <audio>
  // element to 0 mid-listen.
  it('should not mint a new stream URL when the window regains focus', async () => {
    vi.mocked(apiClient.getStreamUrl).mockResolvedValue({
      url: '/api/v1/media/media-1/stream?token=abc',
      expires_at: '2026-03-01T00:00:00Z',
    })
    // The media row alongside it is the control: it takes the client's default
    // refetch-on-focus, so if focus were not reaching the cache at all this
    // test would prove nothing.
    vi.mocked(apiClient.get).mockResolvedValue({ id: 'media-1', status: 'ready' })

    const { result } = renderHook(
      () => ({ stream: useMediaStreamUrl('media-1'), detail: useMediaDetail('media-1') }),
      { wrapper: createWrapper() },
    )
    await waitFor(() => {
      expect(result.current.stream.isSuccess).toBe(true)
      expect(result.current.detail.isSuccess).toBe(true)
    })
    expect(apiClient.getStreamUrl).toHaveBeenCalledTimes(1)
    expect(apiClient.get).toHaveBeenCalledTimes(1)

    act(() => {
      focusManager.setFocused(false)
      focusManager.setFocused(true)
    })
    await waitFor(() => {
      expect(apiClient.get).toHaveBeenCalledTimes(2)
    })

    expect(apiClient.getStreamUrl).toHaveBeenCalledTimes(1)
    focusManager.setFocused(undefined)
  })

  it('should not fetch stream URL when media id is empty', () => {
    const { result } = renderHook(() => useMediaStreamUrl(''), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(apiClient.getStreamUrl).not.toHaveBeenCalled()
  })

  it('should not retry on scan-blocked errors', async () => {
    vi.mocked(apiClient.getStreamUrl).mockRejectedValueOnce(new ApiError(409, 'scan pending'))

    const { result } = renderHook(() => useMediaStreamUrl('media-1'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(apiClient.getStreamUrl).toHaveBeenCalledTimes(1)
  })
})

describe('useUpdateMedia', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should call PATCH /media/:id', async () => {
    const mockMedia = {
      id: 'media-1',
      filename: 'recording.mp3',
      content_type: 'audio/mpeg',
      size: 1024000,
      duration: 120,
      status: 'ready',
      title: 'New Title',
      created_at: '2026-01-15T10:00:00Z',
    }

    vi.mocked(apiClient.patch).mockResolvedValueOnce(mockMedia)

    const { result } = renderHook(() => useUpdateMedia(), { wrapper: createWrapper() })

    await act(async () => {
      result.current.mutate({ id: 'media-1', data: { title: 'New Title' } })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(apiClient.patch).toHaveBeenCalledWith('/media/media-1', { title: 'New Title' })
  })
})

describe('useDeleteMedia', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should call delete endpoint', async () => {
    vi.mocked(apiClient.delete).mockResolvedValueOnce(null)
    const queryClient = createTestQueryClient()
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useDeleteMedia(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate('media-to-delete')
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.delete).toHaveBeenCalledWith('/media/media-to-delete')
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['usage', 'stats'] })
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['media'] })
    // The server cascades into transcriptions and summaries, so their cached
    // lists still name rows that no longer exist.
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['transcriptions'] })
  })
})
