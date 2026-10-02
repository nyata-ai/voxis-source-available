import { describe, expect, it } from 'vitest'
import {
  getActivityRefetchInterval,
  optimisticRecordingItem,
  optimisticTranscriptionItem,
} from './useActivity'

describe('Voxis-OSS activity helpers', () => {
  it('uses the normal transcription route for newly started work', () => {
    expect(optimisticTranscriptionItem('tx-1', '2026-09-13T00:00:00.000Z')).toMatchObject({
      kind: 'transcription',
      link: '/transcriptions/tx-1',
    })
  })

  it('keeps the fast polling interval while work is active', () => {
    expect(
      getActivityRefetchInterval([
        { ...optimisticRecordingItem('rec-1', '2026-09-13T00:00:00.000Z') },
      ])
    ).toBe(3_000)
  })
})
