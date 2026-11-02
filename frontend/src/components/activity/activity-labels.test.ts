import { describe, expect, it } from 'vitest'
import type { ActivityItem } from '@/types/activity'
import { activityIntake, activityStageKey, activitySteps } from './activity-labels'

function item(overrides: Partial<ActivityItem>): ActivityItem {
  return {
    kind: 'transcription',
    ref_id: 'tr-1',
    link: '/transcriptions/tr-1',
    stage: 'transcribing',
    status: 'in_progress',
    started_at: '2026-11-02T00:00:00Z',
    ...overrides,
  }
}

describe('activity labels', () => {
  it('maps stages to common namespace activity keys', () => {
    expect(activityStageKey('stitching')).toBe('activity.stage.stitching')
    expect(activityStageKey('summarizing')).toBe('activity.stage.summarizing')
  })
})

describe('activitySteps', () => {
  it('draws Saving → Ready for a recording', () => {
    expect(activitySteps(item({ kind: 'recording', stage: 'stitching' }))).toEqual(['current', 'pending'])
    expect(activitySteps(item({ kind: 'recording', stage: 'ready', status: 'completed' }))).toEqual(['done', 'done'])
    expect(activitySteps(item({ kind: 'recording', status: 'failed' }))).toEqual(['failed', 'pending'])
  })

  it('leaves the briefing step out until briefings are running', () => {
    expect(activitySteps(item({}))).toEqual(['current', 'pending'])
    expect(
      activitySteps(item({ stage: 'summarizing', detail: { terminal: 2, total: 4 } })),
    ).toEqual(['done', 'current', 'pending'])
  })

  it('fills every step when the job is ready and marks a failure in place', () => {
    expect(activitySteps(item({ stage: 'ready', status: 'completed' }))).toEqual(['done', 'done'])
    expect(activitySteps(item({ status: 'failed' }))).toEqual(['failed', 'pending'])
  })
})

describe('activityIntake', () => {
  it('tells record, upload and link apart', () => {
    expect(activityIntake(item({ kind: 'recording', link: undefined }))).toBe('record')
    expect(activityIntake(item({}))).toBe('upload')
    expect(activityIntake(item({ link: '/url-transcriptions/tr-1' }))).toBe('link')
  })
})
