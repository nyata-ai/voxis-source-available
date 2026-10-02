import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'

vi.mock('@/lib/api-client', () => ({
  apiClient: {
    get: vi.fn(),
  },
}))

import { apiClient } from '@/lib/api-client'
import { useUsageStats } from './useUsageStats'

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children)
  }
}

describe('useUsageStats', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('should return usage stats when query succeeds', async () => {
    const mockStats = {
      total_duration_seconds: 3661.5,
      total_prompt_tokens: 150000,
      total_completion_tokens: 45000,
      total_thinking_tokens: 12000,
      url_transcriptions_remaining_today: 3,
      live_recording_retention_enabled: true,
      live_recording_retention_days: 7,
      storage_used_bytes: 12582912,
    }
    vi.mocked(apiClient.get).mockResolvedValueOnce(mockStats)

    const { result } = renderHook(() => useUsageStats(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data).toEqual(mockStats)
    expect(apiClient.get).toHaveBeenCalledWith('/usage/stats')
  })

  it('should handle error', async () => {
    const error = new Error('Network error')
    vi.mocked(apiClient.get).mockRejectedValueOnce(error)

    const { result } = renderHook(() => useUsageStats(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(result.current.error).toBe(error)
  })

  it('should start in loading state', () => {
    vi.mocked(apiClient.get).mockReturnValue(new Promise(() => {}))

    const { result } = renderHook(() => useUsageStats(), {
      wrapper: createWrapper(),
    })

    expect(result.current.isLoading).toBe(true)
    expect(result.current.data).toBeUndefined()
  })
})
