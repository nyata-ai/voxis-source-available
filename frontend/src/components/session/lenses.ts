import { SUMMARY_TYPE_ORDER } from '@/components/summary/SummaryTypeMeta'
import type { SummaryDetail, SummaryType } from '@/types/summary'
import type { SessionLens } from './session-view'

// Status ranking used to pick the representative row per summary type. A type
// can carry several non-deleted rows: "Generate missing summaries" inserts a
// fresh row beside an older `failed` one (the active-row unique index excludes
// `failed`/`deleted`), so the list returns both. Without this, last-write-wins
// over the created_at-DESC list would surface the stale failure after a retry.
const SUMMARY_STATUS_RANK: Record<string, number> = { completed: 3, pending: 2, failed: 1 }

function rankSummary(summary: SummaryDetail): number {
  return SUMMARY_STATUS_RANK[summary.status] ?? 0
}

// Prefer the higher-ranked status; break ties by the newer created_at.
function isPreferredSummary(candidate: SummaryDetail, current: SummaryDetail): boolean {
  const candidateRank = rankSummary(candidate)
  const currentRank = rankSummary(current)
  if (candidateRank !== currentRank) return candidateRank > currentRank
  return (candidate.created_at ?? '') > (current.created_at ?? '')
}

/** Deduplicates a raw summary list down to one representative row per type. */
export function toSummaryTypeMap(
  summaries: SummaryDetail[]
): Partial<Record<SummaryType, SummaryDetail>> {
  const result: Partial<Record<SummaryType, SummaryDetail>> = {}
  for (const summary of summaries) {
    const current = result[summary.summary_type]
    if (!current || isPreferredSummary(summary, current)) {
      result[summary.summary_type] = summary
    }
  }
  return result
}

/**
 * Projects a session's summaries onto the four fixed briefing lenses, always in
 * `SUMMARY_TYPE_ORDER` and always four entries long — a never-generated type is
 * a lens with no `summaryId`, not a missing tab.
 *
 * `summary` is populated only when the caller's rows already carry content (URL
 * detail responses embed it). Content-less rows — what
 * `GET /transcriptions/:id/summaries` returns — leave it undefined so the pane
 * fetches per lens by `summaryId`.
 */
export function toLenses(summaries: SummaryDetail[]): SessionLens[] {
  const byType = toSummaryTypeMap(summaries)
  return SUMMARY_TYPE_ORDER.map((type) => {
    const representative = byType[type]
    if (!representative) return { type }
    return {
      type,
      summaryId: representative.id,
      status: representative.status,
      summary: representative.content !== undefined ? representative : undefined,
    }
  })
}
