import { describe, expect, it } from 'vitest'
import type { Utterance } from '@/types/transcription'
import { MAX_LANES, layoutStrip, seekTimeInSegment, turnAt } from './speaker-strip'

function turn(speaker: number, start: number, end: number): Utterance {
  return { id: start, speaker, start, end, text: 'x', words: [] }
}

describe('turnAt', () => {
  it('finds the turn under the time, and nothing in a gap', () => {
    const turns = [turn(0, 0, 10), turn(1, 20, 30)]
    expect(turnAt(turns, 5)?.speaker).toBe(0)
    expect(turnAt(turns, 25)?.speaker).toBe(1)
    expect(turnAt(turns, 15)).toBeUndefined()
    expect(turnAt(turns, 30)).toBeUndefined()
  })
})

describe('seekTimeInSegment', () => {
  const turns = [turn(0, 0, 10), turn(1, 10, 20), turn(0, 20, 30), turn(1, 30, 40), turn(0, 40, 50)]
  const segment = { start: 0, end: 50 }

  it('snaps to the lane speaker\'s turn at that time, ignoring other lanes', () => {
    expect(seekTimeInSegment(turns, [0], segment, 0.5)).toBe(20)
    expect(seekTimeInSegment(turns, [0], segment, 0.7)).toBe(20)
    expect(seekTimeInSegment(turns, [0], segment, 0.9)).toBe(40)
  })

  it('stays inside the segment at its edges and past them', () => {
    expect(seekTimeInSegment(turns, [0], segment, 0)).toBe(0)
    expect(seekTimeInSegment(turns, [0], segment, -3)).toBe(0)
    expect(seekTimeInSegment(turns, [0], segment, 9)).toBe(40)
  })

  it('leaves a single-turn segment at its start', () => {
    expect(seekTimeInSegment(turns, [1], { start: 10, end: 20 }, 0.8)).toBe(10)
  })

  it('counts every folded speaker in a shared lane', () => {
    expect(seekTimeInSegment(turns, [0, 1], segment, 0.5)).toBe(20)
    expect(seekTimeInSegment(turns, [0, 1], segment, 0.65)).toBe(30)
  })
})

describe('layoutStrip', () => {
  it('gives each speaker a lane, in speaker order', () => {
    const layout = layoutStrip([turn(1, 0, 10), turn(0, 10, 20)], 20, 200)
    expect(layout.lanes.map((lane) => lane.speakers)).toEqual([[0], [1]])
    expect(layout.lanes[1]!.segments[0]).toMatchObject({ speaker: 1, start: 0, x: 0, w: 100 })
  })

  it('folds six or more speakers into a shared last lane, keeping the busiest', () => {
    // Speakers 0-5; speaker 5 talks most, speakers 3 and 4 least.
    const turns = [
      turn(0, 0, 10),
      turn(1, 10, 20),
      turn(2, 20, 30),
      turn(3, 30, 31),
      turn(4, 31, 32),
      turn(5, 32, 60),
    ]
    const layout = layoutStrip(turns, 60, 600)
    expect(layout.lanes).toHaveLength(MAX_LANES)
    expect(layout.lanes.map((lane) => lane.speakers)).toEqual([[0], [1], [2], [5], [3, 4]])
  })

  it('merges turns closer than a pixel, so the work is bounded by the width', () => {
    // A two-hour hearing: 7,200 one-second turns alternating two speakers.
    const turns = Array.from({ length: 7200 }, (_, i) => turn(i % 2, i, i + 0.9))
    const width = 320
    const layout = layoutStrip(turns, 7200, width)
    for (const lane of layout.lanes) {
      expect(lane.segments.length).toBeLessThanOrEqual(width)
    }
    expect(layout.lanes[0]!.segments).toHaveLength(1)
  })

  it('keeps wide turns by different folded speakers apart in the shared lane', () => {
    // Speakers 4-6 fold; each talks long enough to draw, back to back.
    const turns = [
      turn(0, 0, 100),
      turn(1, 100, 200),
      turn(2, 200, 300),
      turn(3, 300, 400),
      turn(4, 400, 410),
      turn(5, 410, 420),
      turn(6, 420, 430),
    ]
    const shared = layoutStrip(turns, 430, 430).lanes[4]!
    expect(shared.speakers).toEqual([4, 5, 6])
    expect(shared.segments.map((segment) => segment.speaker)).toEqual([4, 5, 6])
  })

  it('still bounds the shared lane by the width when folded turns are thin', () => {
    const turns = Array.from({ length: 7200 }, (_, i) => turn(i % 8, i, i + 1))
    const width = 320
    for (const lane of layoutStrip(turns, 7200, width).lanes) {
      expect(lane.segments.length).toBeLessThanOrEqual(width)
    }
  })

  it('keeps a gap of a pixel or more as two segments', () => {
    const layout = layoutStrip([turn(0, 0, 10), turn(0, 20, 30)], 30, 300)
    expect(layout.lanes[0]!.segments).toHaveLength(2)
  })

  it('drops a lone turn too short to draw honestly', () => {
    const layout = layoutStrip([turn(0, 0, 100), turn(1, 200, 200.1)], 1000, 100)
    expect(layout.lanes[1]!.segments).toHaveLength(0)
  })

  it('totals speaking time for the summary', () => {
    const layout = layoutStrip([turn(0, 0, 30), turn(1, 30, 40)], 40, 100)
    expect(layout.talkTime.get(0)).toBe(30)
    expect(layout.totalTalk).toBe(40)
  })

  it('draws nothing without a duration, a width or turns', () => {
    expect(layoutStrip([turn(0, 0, 1)], 0, 100).lanes).toEqual([])
    expect(layoutStrip([turn(0, 0, 1)], 10, 0).lanes).toEqual([])
    expect(layoutStrip([], 10, 100).lanes).toEqual([])
  })
})
