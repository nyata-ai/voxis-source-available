import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'
import { normalizeAdminSystemStats, useAdminSystemStats } from './useAdminSystemStats'
import { apiClient } from '@/lib/api-client'

vi.mock('@/lib/api-client', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api-client')>('@/lib/api-client')
  return {
    ...actual,
    apiClient: {
      get: vi.fn(),
    },
  }
})

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

describe('useAdminSystemStats', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('fetches aggregate admin system statistics', async () => {
    const stats = {
      generated_at: '2026-06-25T01:00:00Z',
      host: {
        available: true,
        ip: '192.0.2.10',
        external_ip: '198.51.100.20',
        cpu: { load_1: 0.42, load_5: 0.55, load_15: 0.61, cores: 4 },
        memory: { used_bytes: 12582912, available_bytes: 4194304, total_bytes: 16777216 },
        uptime: { host_seconds: 123456, process_seconds: 90 },
      },
      build: { available: true, version: '1.2.3', commit: 'abc1234', build_time: '2026-06-24T00:00:00Z' },
      disk: { available: true, mount: '/', used_bytes: 245760, free_bytes: 122880, total_bytes: 409600 },
      storage: {
        available: true,
        backend: 'gcs',
        gcs: { bucket: 'example-media-bucket', has_data: true, total_bytes: 1200000, object_count: 48210, as_of: '2026-06-25T03:00:00Z' },
      },
      database: {
        available: true,
        size_bytes: 5242880,
        pool: { acquired: 3, idle: 4, max: 25, total: 7 },
      },
      users: { available: true, total: 1284, organizations: 37, new_7d: 12, new_30d: 58 },
      dependencies: [
        { name: 'postgres', status: 'up', latency_ms: 2 },
        { name: 'clamav', status: 'down', latency_ms: 0, reason: 'connection refused' },
      ],
    }

    vi.mocked(apiClient.get).mockResolvedValueOnce(stats)

    const { result } = renderHook(() => useAdminSystemStats(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data).toEqual(stats)
    expect(apiClient.get).toHaveBeenCalledWith('/admin/system/stats')
  })

  it('normalizes missing sections to safe defaults', () => {
    const normalized = normalizeAdminSystemStats({})

    expect(normalized.host.available).toBe(false)
    expect(normalized.dependencies).toEqual([])
    expect(normalized.storage.backend).toBe('unknown')
    expect(normalized.users.total).toBe(0)
  })

  it('returns safe defaults for null or undefined response bodies', () => {
    for (const input of [null, undefined] as const) {
      expect(() => normalizeAdminSystemStats(input)).not.toThrow()

      const normalized = normalizeAdminSystemStats(input)
      expect(normalized.host.available).toBe(false)
      expect(normalized.dependencies).toEqual([])
      expect(normalized.storage.backend).toBe('unknown')
    }
  })
})
