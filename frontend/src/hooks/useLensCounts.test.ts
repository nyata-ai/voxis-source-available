import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useLensCounts, LENS_FETCH_LIMIT } from './useLensCounts'
import { useSummaryList } from '@/hooks/useSummary'
import type { SummaryItem, SummaryListResponse } from '@/types/summary'

vi.mock('@/hooks/useSummary', () => ({
  useSummaryList: vi.fn(),
}))

const mockUseSummaryList = vi.mocked(useSummaryList)

function summary(overrides: Partial<SummaryItem> = {}): SummaryItem {
  return {
    id: 'sum-1',
    organization_id: 'org-1',
    transcription_id: 'tx-1',
    summary_type: 'general',
    status: 'completed',
    word_count: 10,
    prompt_tokens: 1,
    completion_tokens: 1,
    high_stakes: false,
    review_status: 'passed',
    created_at: '2026-02-10T10:00:00Z',
    ...overrides,
  }
}

function queryState(overrides: Partial<{
  data: SummaryListResponse
  isLoading: boolean
  isError: boolean
}> = {}) {
  return {
    data: undefined,
    isLoading: false,
    isError: false,
    ...overrides,
  } as ReturnType<typeof useSummaryList>
}

describe('useLensCounts', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns an empty map while loading, and never throws', () => {
    mockUseSummaryList.mockReturnValue(queryState({ isLoading: true }))

    const { result } = renderHook(() => useLensCounts())

    expect(result.current.counts.size).toBe(0)
    expect(result.current.clipped).toBe(false)
  })

  it('returns an empty map on error, and never throws', () => {
    mockUseSummaryList.mockReturnValue(queryState({ isError: true }))

    const { result } = renderHook(() => useLensCounts())

    expect(result.current.counts.size).toBe(0)
    expect(result.current.clipped).toBe(false)
  })

  it('groups summaries by transcription_id, counting only completed', () => {
    mockUseSummaryList.mockReturnValue(
      queryState({
        data: {
          items: [
            summary({ id: 's1', transcription_id: 'tx-1', status: 'completed' }),
            summary({ id: 's2', transcription_id: 'tx-1', status: 'completed' }),
            summary({ id: 's3', transcription_id: 'tx-1', status: 'pending' }),
            summary({ id: 's4', transcription_id: 'tx-2', status: 'completed' }),
            summary({ id: 's5', transcription_id: 'tx-2', status: 'failed' }),
          ],
          total: 5,
        },
      })
    )

    const { result } = renderHook(() => useLensCounts())

    expect(result.current.counts.get('tx-1')).toBe(2)
    expect(result.current.counts.get('tx-2')).toBe(1)
    expect(result.current.counts.has('tx-3')).toBe(false)
  })

  it('sets clipped true when the server total exceeds the fetched window', () => {
    mockUseSummaryList.mockReturnValue(
      queryState({
        data: { items: [summary()], total: 150 },
      })
    )

    const { result } = renderHook(() => useLensCounts())

    expect(result.current.clipped).toBe(true)
  })

  it('sets clipped false when the fetched window covers the full total', () => {
    mockUseSummaryList.mockReturnValue(
      queryState({
        data: { items: [summary()], total: 1 },
      })
    )

    const { result } = renderHook(() => useLensCounts())

    expect(result.current.clipped).toBe(false)
  })

  it('fetches the server-clamp window (100) unfiltered, unpaginated', () => {
    mockUseSummaryList.mockReturnValue(queryState())

    renderHook(() => useLensCounts())

    expect(LENS_FETCH_LIMIT).toBe(100)
    expect(mockUseSummaryList).toHaveBeenCalledWith(LENS_FETCH_LIMIT, 0, '', { enabled: true })
  })

  it('passes the enabled option through so callers can skip the fetch', () => {
    mockUseSummaryList.mockReturnValue(queryState())

    renderHook(() => useLensCounts(false))

    expect(mockUseSummaryList).toHaveBeenCalledWith(LENS_FETCH_LIMIT, 0, '', { enabled: false })
  })

  it('defaults to enabled', () => {
    mockUseSummaryList.mockReturnValue(queryState())

    renderHook(() => useLensCounts())

    expect(mockUseSummaryList).toHaveBeenCalledWith(LENS_FETCH_LIMIT, 0, '', { enabled: true })
  })
})
