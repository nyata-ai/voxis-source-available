import { describe, expect, it } from 'vitest'
import { activityStageKey, activityStatusTone } from './activity-labels'

describe('activity labels', () => {
  it('maps stages to common namespace activity keys', () => {
    expect(activityStageKey('stitching')).toBe('activity.stage.stitching')
    expect(activityStageKey('summarizing')).toBe('activity.stage.summarizing')
  })

  it('maps statuses to visual tones', () => {
    expect(activityStatusTone('in_progress')).toBe('spinner')
    expect(activityStatusTone('completed')).toBe('success')
    expect(activityStatusTone('failed')).toBe('error')
  })
})
