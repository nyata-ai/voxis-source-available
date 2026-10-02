import { describe, it, expect } from 'vitest'
import { toLenses } from './lenses'
import { SUMMARY_TYPE_ORDER } from '@/components/summary/SummaryTypeMeta'
import type { SummaryDetail, SummaryType } from '@/types/summary'

function summary(overrides: Partial<SummaryDetail> & { id: string }): SummaryDetail {
  return {
    organization_id: 'org-1',
    transcription_id: 'tx-1',
    summary_type: 'general',
    status: 'completed',
    word_count: 10,
    prompt_tokens: 100,
    completion_tokens: 50,
    high_stakes: false,
    review_status: 'skipped',
    created_at: '2026-08-01T10:00:00Z',
    ...overrides,
  }
}

describe('toLenses', () => {
  it('returns exactly four lenses in SUMMARY_TYPE_ORDER', () => {
    const lenses = toLenses([summary({ id: 's1', summary_type: 'action_items' })])

    expect(lenses).toHaveLength(4)
    expect(lenses.map((lens) => lens.type)).toEqual(SUMMARY_TYPE_ORDER)
  })

  it('leaves summaryId undefined for never-generated types', () => {
    const lenses = toLenses([summary({ id: 's1', summary_type: 'general' })])

    const byType = new Map(lenses.map((lens) => [lens.type, lens]))
    expect(byType.get('general')?.summaryId).toBe('s1')
    expect(byType.get('general')?.status).toBe('completed')
    for (const type of ['key_points', 'action_items', 'q_and_a'] as SummaryType[]) {
      expect(byType.get(type)?.summaryId).toBeUndefined()
      expect(byType.get(type)?.status).toBeUndefined()
    }
  })

  it('prefers a fresh completed row over a stale failed one of the same type', () => {
    const lenses = toLenses([
      summary({
        id: 'stale',
        summary_type: 'general',
        status: 'failed',
        created_at: '2026-08-05T10:00:00Z',
      }),
      summary({
        id: 'fresh',
        summary_type: 'general',
        status: 'completed',
        created_at: '2026-08-01T10:00:00Z',
      }),
    ])

    const general = lenses.find((lens) => lens.type === 'general')
    expect(general?.summaryId).toBe('fresh')
    expect(general?.status).toBe('completed')
  })

  it('breaks same-status ties by the newer created_at', () => {
    const lenses = toLenses([
      summary({ id: 'older', summary_type: 'key_points', created_at: '2026-08-01T10:00:00Z' }),
      summary({ id: 'newer', summary_type: 'key_points', created_at: '2026-08-06T10:00:00Z' }),
    ])

    expect(lenses.find((lens) => lens.type === 'key_points')?.summaryId).toBe('newer')
  })

  it('carries embedded content through as `summary` and omits it when content-less', () => {
    const lenses = toLenses([
      summary({ id: 'with', summary_type: 'general', content: 'Body text.' }),
      summary({ id: 'without', summary_type: 'key_points' }),
    ])

    const byType = new Map(lenses.map((lens) => [lens.type, lens]))
    expect(byType.get('general')?.summary?.id).toBe('with')
    expect(byType.get('key_points')?.summary).toBeUndefined()
  })
})
