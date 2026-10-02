import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'
import {
  adminRecordingRetentionKeys,
  useAdminRecordingRetentionPolicy,
  useAdminRecordingRetentionPreview,
  useUpdateAdminRecordingRetentionPolicy,
} from './useAdminRecordingRetention'
import { apiClient } from '@/lib/api-client'

vi.mock('@/lib/api-client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
  },
}))

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children)
  }
}

describe('adminRecordingRetentionKeys', () => {
  it('uses the admin namespace', () => {
    expect(adminRecordingRetentionKeys.policy()).toEqual(['admin', 'recording-retention', 'policy'])
  })
})

describe('admin recording retention hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('fetches the global recording retention policy from admin API', async () => {
    const policy = { enabled: false, days: 7, apply_to_existing: false }
    vi.mocked(apiClient.get).mockResolvedValueOnce(policy)

    const { result } = renderHook(() => useAdminRecordingRetentionPolicy(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toEqual(policy)
    expect(apiClient.get).toHaveBeenCalledWith('/admin/recording-retention')
  })

  it('previews the global recording retention policy from admin API', async () => {
    const preview = {
      immediate_delete_count: 3,
      oldest_completed_at: '2026-06-01T00:00:00Z',
      prospective_effective_at: '2026-06-30T00:00:00Z',
    }
    vi.mocked(apiClient.post).mockResolvedValueOnce(preview)

    const { result } = renderHook(() => useAdminRecordingRetentionPreview(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ enabled: true, days: 7, apply_to_existing: true })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(apiClient.post).toHaveBeenCalledWith('/admin/recording-retention/preview', {
      enabled: true,
      days: 7,
      apply_to_existing: true,
    })
    expect(result.current.data).toEqual(preview)
  })

  it('updates the global policy and invalidates it', async () => {
    const policy = {
      enabled: true,
      days: 14,
      apply_to_existing: false,
      effective_at: '2026-06-30T00:00:00Z',
    }
    vi.mocked(apiClient.put).mockResolvedValueOnce(policy)

    const { result } = renderHook(() => useUpdateAdminRecordingRetentionPolicy(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        enabled: true,
        days: 14,
        apply_to_existing: false,
        confirmed_immediate_delete_count: 0,
      })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(apiClient.put).toHaveBeenCalledWith('/admin/recording-retention', {
      enabled: true,
      days: 14,
      apply_to_existing: false,
      confirmed_immediate_delete_count: 0,
    })
  })
})
