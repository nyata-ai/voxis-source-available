import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'
import { useAdmin, adminKeys } from './useAdmin'
import { apiClient, ApiError } from '@/lib/api-client'

vi.mock('@/lib/api-client', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api-client')>('@/lib/api-client')
  return {
    ...actual,
    apiClient: {
      get: vi.fn(),
    },
  }
})

vi.mock('@/contexts/AuthContext', () => ({
  useAuth: () => ({
    user: { id: 'user-123' },
  }),
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

function createRetryingWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: 3,
        retryDelay: 1,
        gcTime: 0,
      },
    },
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children)
  }
}

describe('useAdmin', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('fetches the admin capability contract from /admin/me', async () => {
    const adminMe = {
      admin: true,
    }

    vi.mocked(apiClient.get).mockResolvedValueOnce(adminMe)

    const { result } = renderHook(() => useAdmin(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data).toEqual(adminMe)
    expect(apiClient.get).toHaveBeenCalledTimes(1)
    expect(apiClient.get).toHaveBeenCalledWith('/admin/me')
  })

  it('surfaces a 403 response for callers that need a forbidden state', async () => {
    const error = new ApiError(403, 'Forbidden')
    vi.mocked(apiClient.get).mockRejectedValueOnce(error)

    const { result } = renderHook(() => useAdmin(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(result.current.error).toBe(error)
  })

  it('does not retry authorization failures', async () => {
    const error = new ApiError(403, 'Forbidden')
    vi.mocked(apiClient.get).mockRejectedValue(error)

    const { result } = renderHook(() => useAdmin(), {
      wrapper: createRetryingWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(result.current.error).toBe(error)
    expect(apiClient.get).toHaveBeenCalledTimes(1)
  })

  it('bounds retries for server failures', async () => {
    const error = new ApiError(500, 'Server error')
    vi.mocked(apiClient.get).mockRejectedValue(error)

    const { result } = renderHook(() => useAdmin(), {
      wrapper: createRetryingWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(result.current.error).toBe(error)
    expect(apiClient.get).toHaveBeenCalledTimes(4)
  })

  it('keys the local admin status cache by current authenticated user', () => {
    expect(adminKeys.me('user-123')).toEqual(['admin', 'me', 'user-123'])
  })
})
