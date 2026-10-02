import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'
import { useCurrentUser } from './useCurrentUser'
import { apiClient } from '@/lib/api-client'

vi.mock('@/lib/api-client', () => ({
  apiClient: {
    get: vi.fn(),
  },
}))

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

describe('useCurrentUser', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should fetch current user data', async () => {
    const mockUser = {
      id: 'user-123',
      email: 'test@example.com',
      name: 'Test User',
      organization: {
        id: 'org-456',
        name: 'Test Org',
        tier: 'free',
      },
    }

    vi.mocked(apiClient.get).mockResolvedValueOnce(mockUser)

    const { result } = renderHook(() => useCurrentUser(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data).toEqual(mockUser)
    expect(apiClient.get).toHaveBeenCalledWith('/me')
  })

  it('should handle loading state', () => {
    vi.mocked(apiClient.get).mockImplementation(
      () => new Promise(() => {}) // Never resolves
    )

    const { result } = renderHook(() => useCurrentUser(), {
      wrapper: createWrapper(),
    })

    expect(result.current.isLoading).toBe(true)
    expect(result.current.data).toBeUndefined()
  })

  it('should handle error state', async () => {
    const error = new Error('API Error')
    vi.mocked(apiClient.get).mockRejectedValueOnce(error)

    const { result } = renderHook(() => useCurrentUser(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(result.current.error).toBe(error)
  })

  it('should handle user without organization', async () => {
    const mockUser = {
      id: 'user-123',
      email: 'solo@example.com',
      name: 'Solo User',
    }

    vi.mocked(apiClient.get).mockResolvedValueOnce(mockUser)

    const { result } = renderHook(() => useCurrentUser(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data).toEqual(mockUser)
    expect(result.current.data?.organization).toBeUndefined()
  })

  it('should call the correct API endpoint', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({
      id: 'user-1',
      email: 'test@example.com',
      name: 'Test',
    })

    const { result } = renderHook(() => useCurrentUser(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.get).toHaveBeenCalledTimes(1)
    expect(apiClient.get).toHaveBeenCalledWith('/me')
  })
})
