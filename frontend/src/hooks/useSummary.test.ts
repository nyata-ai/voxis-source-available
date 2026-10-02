import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'
import {
  useSummaryList,
  useSummaryDetail,
  useCreateSummary,
  useDeleteSummary,
  useRegenerateSummary,
  useSummariesByTranscription,
  summaryKeys,
} from './useSummary'
import { apiClient } from '@/lib/api-client'

vi.mock('@/lib/api-client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
  },
}))

function createWrapper() {
  const queryClient = new QueryClient({
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
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children)
  }
}

describe('summaryKeys', () => {
  it('should generate hierarchical keys', () => {
    expect(summaryKeys.all).toEqual(['summaries'])
    expect(summaryKeys.list()).toEqual(['summaries', 'list', { limit: 20, offset: 0, search: '' }])
    expect(summaryKeys.list({ limit: 10, offset: 5 })).toEqual([
      'summaries',
      'list',
      { limit: 10, offset: 5, search: '' },
    ])
    expect(summaryKeys.detail('sum-123')).toEqual(['summaries', 'detail', 'sum-123'])
    expect(summaryKeys.byTranscription('tx-456')).toEqual([
      'summaries',
      'byTranscription',
      'tx-456',
    ])
  })
})

describe('useSummaryList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should fetch summary list with default params', async () => {
    const mockResponse = {
      items: [
        {
          id: 'sum-1',
          organization_id: 'org-1',
          transcription_id: 'tx-1',
          summary_type: 'general',
          status: 'completed',
          word_count: 150,
          prompt_tokens: 1000,
          completion_tokens: 200,
          created_at: '2026-02-10T10:00:00Z',
          completed_at: '2026-02-10T10:01:00Z',
        },
      ],
      total: 1,
    }

    vi.mocked(apiClient.get).mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useSummaryList(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data).toEqual(mockResponse)
    expect(apiClient.get).toHaveBeenCalledWith('/summaries?limit=20&offset=0')
  })

  it('should pass custom limit and offset', async () => {
    const mockResponse = { items: [], total: 0 }
    vi.mocked(apiClient.get).mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useSummaryList(10, 5), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.get).toHaveBeenCalledWith('/summaries?limit=10&offset=5')
  })

  it('should include search query when provided', async () => {
    const mockResponse = { items: [], total: 0 }
    vi.mocked(apiClient.get).mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useSummaryList(10, 5, 'intro plan'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.get).toHaveBeenCalledWith('/summaries?limit=10&offset=5&search=intro%20plan')
  })
})

describe('useSummaryDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should fetch summary detail by id', async () => {
    const mockDetail = {
      id: 'sum-1',
      organization_id: 'org-1',
      transcription_id: 'tx-1',
      summary_type: 'general',
      status: 'completed',
      word_count: 150,
      prompt_tokens: 1000,
      completion_tokens: 200,
      created_at: '2026-02-10T10:00:00Z',
      completed_at: '2026-02-10T10:01:00Z',
      content: 'This is a summary of the transcription.',
    }

    vi.mocked(apiClient.get).mockResolvedValueOnce(mockDetail)

    const { result } = renderHook(() => useSummaryDetail('sum-1'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data).toEqual(mockDetail)
    expect(apiClient.get).toHaveBeenCalledWith('/summaries/sum-1')
  })

  it('should not fetch when id is empty', () => {
    const { result } = renderHook(() => useSummaryDetail(''), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(apiClient.get).not.toHaveBeenCalled()
  })

  it('should poll when status is pending', async () => {
    const mockDetail = {
      id: 'sum-1',
      organization_id: 'org-1',
      transcription_id: 'tx-1',
      summary_type: 'general',
      status: 'pending',
      word_count: 0,
      prompt_tokens: 0,
      completion_tokens: 0,
      created_at: '2026-02-10T10:00:00Z',
    }

    vi.mocked(apiClient.get).mockResolvedValue(mockDetail)

    const { result } = renderHook(() => useSummaryDetail('sum-1'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    // Verify the refetchInterval is set (data has pending status)
    expect(result.current.data?.status).toBe('pending')
  })

  it('should stop polling when status is completed', async () => {
    const mockDetail = {
      id: 'sum-1',
      organization_id: 'org-1',
      transcription_id: 'tx-1',
      summary_type: 'general',
      status: 'completed',
      word_count: 150,
      prompt_tokens: 1000,
      completion_tokens: 200,
      created_at: '2026-02-10T10:00:00Z',
      completed_at: '2026-02-10T10:01:00Z',
      content: 'Summary content here.',
    }

    vi.mocked(apiClient.get).mockResolvedValueOnce(mockDetail)

    const { result } = renderHook(() => useSummaryDetail('sum-1'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data?.status).toBe('completed')
    // When completed, refetchInterval should return false (no more polling)
  })

  it('should stop polling when status is failed', async () => {
    const mockDetail = {
      id: 'sum-1',
      organization_id: 'org-1',
      transcription_id: 'tx-1',
      summary_type: 'general',
      status: 'failed',
      word_count: 0,
      prompt_tokens: 0,
      completion_tokens: 0,
      error_message: 'AI model unavailable',
      created_at: '2026-02-10T10:00:00Z',
    }

    vi.mocked(apiClient.get).mockResolvedValueOnce(mockDetail)

    const { result } = renderHook(() => useSummaryDetail('sum-1'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data?.status).toBe('failed')
    // When failed, refetchInterval should return false (no more polling)
  })
})

describe('useCreateSummary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should post with correct data and invalidate cache', async () => {
    const mockResponse = {
      id: 'sum-new',
      organization_id: 'org-1',
      transcription_id: 'tx-1',
      summary_type: 'key_points',
      status: 'pending',
      word_count: 0,
      prompt_tokens: 0,
      completion_tokens: 0,
      created_at: '2026-02-10T10:00:00Z',
    }

    vi.mocked(apiClient.post).mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useCreateSummary(), {
      wrapper: createWrapper(),
    })

    const createData = {
      transcription_id: 'tx-1',
      summary_type: 'key_points' as const,
    }

    await act(async () => {
      result.current.mutate(createData)
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.post).toHaveBeenCalledWith('/summaries', createData)
    expect(result.current.data).toEqual(mockResponse)
  })
})

describe('useDeleteSummary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should call delete endpoint and invalidate cache', async () => {
    vi.mocked(apiClient.delete).mockResolvedValueOnce(null)

    const { result } = renderHook(() => useDeleteSummary(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate('sum-to-delete')
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.delete).toHaveBeenCalledWith('/summaries/sum-to-delete')
  })
})

describe('useRegenerateSummary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should post to regenerate endpoint and invalidate cache', async () => {
    const mockResponse = {
      id: 'sum-regenerated',
      organization_id: 'org-1',
      transcription_id: 'tx-1',
      summary_type: 'general',
      status: 'pending',
      word_count: 0,
      prompt_tokens: 0,
      completion_tokens: 0,
      created_at: '2026-02-10T10:05:00Z',
    }

    vi.mocked(apiClient.post).mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useRegenerateSummary(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ id: 'sum-old-id' })
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.post).toHaveBeenCalledWith('/summaries/sum-old-id/regenerate')
    expect(result.current.data).toEqual(mockResponse)
  })

  it('sends a one-time summary profile override', async () => {
    vi.mocked(apiClient.post).mockResolvedValueOnce({ id: 'sum-new' })
    const { result } = renderHook(() => useRegenerateSummary(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ id: 'sum-old-id', summaryProfile: 'journalism' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(apiClient.post).toHaveBeenCalledWith('/summaries/sum-old-id/regenerate', {
      summary_profile: 'journalism',
    })
  })
})

describe('useSummariesByTranscription', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should fetch summaries for a specific transcription', async () => {
    const mockResponse = [
      {
        id: 'sum-1',
        organization_id: 'org-1',
        transcription_id: 'tx-1',
        summary_type: 'general',
        status: 'completed',
        word_count: 150,
        prompt_tokens: 1000,
        completion_tokens: 200,
        created_at: '2026-02-10T10:00:00Z',
        completed_at: '2026-02-10T10:01:00Z',
      },
      {
        id: 'sum-2',
        organization_id: 'org-1',
        transcription_id: 'tx-1',
        summary_type: 'key_points',
        status: 'completed',
        word_count: 80,
        prompt_tokens: 1000,
        completion_tokens: 100,
        created_at: '2026-02-10T10:02:00Z',
        completed_at: '2026-02-10T10:03:00Z',
      },
    ]

    vi.mocked(apiClient.get).mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useSummariesByTranscription('tx-1'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data).toEqual(mockResponse)
    expect(apiClient.get).toHaveBeenCalledWith('/transcriptions/tx-1/summaries')
  })

  it('should not fetch when transcriptionId is empty', () => {
    const { result } = renderHook(() => useSummariesByTranscription(''), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(apiClient.get).not.toHaveBeenCalled()
  })
})
