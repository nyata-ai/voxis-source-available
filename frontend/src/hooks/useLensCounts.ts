import { useMemo } from 'react'
import { useSummaryList } from '@/hooks/useSummary'
import type { SummaryItem } from '@/types/summary'

// Same stopgap shape as UNIFIED_FETCH_LIMIT in useUnifiedTranscriptions: there
// is no server-side "lens count per transcription" endpoint, so this hook
// fetches a window of `GET /summaries` and counts client-side. Unlike
// UNIFIED_FETCH_LIMIT, 100 here is not a client-chosen ceiling to raise later
// — it is the SERVER's hard clamp (`GET /summaries` clamps `limit` to 100
// server-side; asking for more silently returns 100 anyway). If an org's
// total summary count exceeds this window, `clipped` below goes true and any
// transcription outside the window reads as `undefined` (not `0`) in
// `counts` — treat that as "unknown", not "no lenses", and prefer a real
// backend count endpoint over raising this further if clipping shows up.
export const LENS_FETCH_LIMIT = 100

export interface LensCounts {
  /** transcription_id -> number of completed summaries, from the fetched window only. */
  counts: Map<string, number>
  /** True when the server's reported total exceeds what this window covered
   * — some transcriptions' lens counts are not represented in `counts`. */
  clipped: boolean
  /** Pass-through of the underlying summary-list query's refetch, so callers
   * (e.g. useLibrarySessions.refetch) can force this join fresh instead of
   * waiting out staleTime. */
  refetch: () => void
}

/**
 * Windowed client-side join: counts completed summaries per transcription so
 * Library rows can show a lens-count chip without a dedicated backend
 * endpoint. Pass `enabled: false` when there is nothing to annotate (e.g. no
 * media-backed rows in the current session list) to skip the fetch entirely.
 * Never throws — loading and error states both resolve to an empty map.
 */
export function useLensCounts(enabled = true): LensCounts {
  const query = useSummaryList(LENS_FETCH_LIMIT, 0, '', { enabled })

  const result = useMemo(() => {
    const counts = new Map<string, number>()
    if (query.isError || !query.data) {
      return { counts, clipped: false }
    }

    const items: SummaryItem[] = query.data.items ?? []
    for (const item of items) {
      if (item.status !== 'completed') continue
      counts.set(item.transcription_id, (counts.get(item.transcription_id) ?? 0) + 1)
    }

    const clipped = (query.data.total ?? 0) > items.length
    return { counts, clipped }
  }, [query.data, query.isError])

  return { ...result, refetch: query.refetch }
}
