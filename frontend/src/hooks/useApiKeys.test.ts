import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'
import { useApiKeys, useCreateApiKey, useRevokeApiKey, apiKeyKeys } from './useApiKeys'
import { apiClient } from '@/lib/api-client'
import type { APIKey, CreateAPIKeyResponse } from '@/types/apikey'

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

describe('apiKeyKeys', () => {
  it('all returns base key', () => {
    expect(apiKeyKeys.all).toEqual(['api-keys'])
  })

  it('list returns list key', () => {
    expect(apiKeyKeys.list()).toEqual(['api-keys', 'list'])
  })
})

describe('useApiKeys', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should fetch api keys and return array from keys field', async () => {
    const mockKeys: APIKey[] = [
      {
        id: 'key-1',
        name: 'My Key',
        key_prefix: 'vx_abc',
        scopes: ['media:read'],
        last_used_at: null,
        expires_at: null,
        created_at: '2026-02-10T10:00:00Z',
      },
    ]

    vi.mocked(apiClient.get).mockResolvedValueOnce({ keys: mockKeys })

    const { result } = renderHook(() => useApiKeys(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data).toEqual(mockKeys)
    expect(apiClient.get).toHaveBeenCalledWith('/api-keys')
  })

  it('should handle network error', async () => {
    const error = new Error('Network error')
    vi.mocked(apiClient.get).mockRejectedValueOnce(error)

    const { result } = renderHook(() => useApiKeys(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(result.current.error).toBe(error)
  })
})

describe('useCreateApiKey', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should post to /api-keys with body', async () => {
    const mockResponse: CreateAPIKeyResponse = {
      id: 'key-new',
      name: 'New Key',
      key: 'vx_abc123full',
      key_prefix: 'vx_abc',
      scopes: ['media:read', 'media:write'],
      mcp_url: 'https://api.voxis.app/mcp',
      api_url: 'https://api.voxis.app/api/v1',
      expires_at: null,
      created_at: '2026-02-12T10:00:00Z',
    }

    vi.mocked(apiClient.post).mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useCreateApiKey(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ name: 'New Key', scopes: ['media:read', 'media:write'] })
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.post).toHaveBeenCalledWith('/api-keys', {
      name: 'New Key',
      scopes: ['media:read', 'media:write'],
    })
    expect(result.current.data).toEqual(mockResponse)
  })

  it('should invalidate api keys query on success', async () => {
    const mockResponse: CreateAPIKeyResponse = {
      id: 'key-new',
      name: 'New Key',
      key: 'vx_abc123full',
      key_prefix: 'vx_abc',
      scopes: ['media:read'],
      mcp_url: 'https://api.voxis.app/mcp',
      api_url: 'https://api.voxis.app/api/v1',
      expires_at: null,
      created_at: '2026-02-12T10:00:00Z',
    }

    // First, populate the cache with a GET
    vi.mocked(apiClient.get).mockResolvedValueOnce({ keys: [] })
    vi.mocked(apiClient.post).mockResolvedValueOnce(mockResponse)

    const wrapper = createWrapper()
    const { result: listResult } = renderHook(() => useApiKeys(), { wrapper })

    await waitFor(() => {
      expect(listResult.current.isSuccess).toBe(true)
    })

    // Now create a key - should trigger refetch
    vi.mocked(apiClient.get).mockResolvedValueOnce({ keys: [{ id: 'key-new', name: 'New Key' }] })

    const { result: createResult } = renderHook(() => useCreateApiKey(), { wrapper })

    await act(async () => {
      createResult.current.mutate({ name: 'New Key', scopes: ['media:read'] })
    })

    await waitFor(() => {
      expect(createResult.current.isSuccess).toBe(true)
    })

    // The GET should have been called twice (initial + invalidation refetch)
    await waitFor(() => {
      expect(apiClient.get).toHaveBeenCalledTimes(2)
    })
  })

  it('should handle 400 validation error', async () => {
    const error = new Error('Bad Request')
    vi.mocked(apiClient.post).mockRejectedValueOnce(error)

    const { result } = renderHook(() => useCreateApiKey(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ name: '', scopes: [] })
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(result.current.error).toBe(error)
  })

  it('should handle 409 limit exceeded error', async () => {
    const error = Object.assign(new Error('Conflict'), { status: 409 })
    vi.mocked(apiClient.post).mockRejectedValueOnce(error)

    const { result } = renderHook(() => useCreateApiKey(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ name: 'Too Many', scopes: ['media:read'] })
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(result.current.error).toBe(error)
  })
})

describe('useRevokeApiKey', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should delete api key by id', async () => {
    vi.mocked(apiClient.delete).mockResolvedValueOnce(null)

    const { result } = renderHook(() => useRevokeApiKey(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate('key-to-revoke')
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.delete).toHaveBeenCalledWith('/api-keys/key-to-revoke')
  })

  it('should invalidate api keys query on success', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({ keys: [{ id: 'key-1' }] })
    vi.mocked(apiClient.delete).mockResolvedValueOnce(null)

    const wrapper = createWrapper()
    const { result: listResult } = renderHook(() => useApiKeys(), { wrapper })

    await waitFor(() => {
      expect(listResult.current.isSuccess).toBe(true)
    })

    vi.mocked(apiClient.get).mockResolvedValueOnce({ keys: [] })

    const { result: revokeResult } = renderHook(() => useRevokeApiKey(), { wrapper })

    await act(async () => {
      revokeResult.current.mutate('key-1')
    })

    await waitFor(() => {
      expect(revokeResult.current.isSuccess).toBe(true)
    })

    await waitFor(() => {
      expect(apiClient.get).toHaveBeenCalledTimes(2)
    })
  })

  it('should handle network error', async () => {
    const error = new Error('Network error')
    vi.mocked(apiClient.delete).mockRejectedValueOnce(error)

    const { result } = renderHook(() => useRevokeApiKey(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate('key-1')
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(result.current.error).toBe(error)
  })
})
