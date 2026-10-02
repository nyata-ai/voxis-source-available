import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { useInterruptedRecording } from './useInterruptedRecording'
import { apiClient } from '@/lib/api-client'

vi.mock('@/lib/api-client', () => ({
  apiClient: {
    get: vi.fn(),
  },
}))

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}

describe('useInterruptedRecording', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('fetches interrupted recording sessions', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({ items: [] })

    const { result } = renderHook(() => useInterruptedRecording(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.get).toHaveBeenCalledWith('/recordings/interrupted')
    expect(result.current.data).toEqual({ items: [] })
  })
})
