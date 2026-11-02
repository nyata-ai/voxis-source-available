import type { Utterance } from '@/types/transcription'

/** Lanes the strip draws at most. With more speakers than this, the busiest
 *  `MAX_LANES - 1` keep their own lane and the rest share a last one. */
export const MAX_LANES = 5

/** A segment narrower than this (in pixels, after merging) is not drawn: at
 *  that width it would be a mark the eye reads as far longer than the speech. */
const MIN_SEGMENT_PX = 0.5

/** Neighbouring turns closer than this many pixels merge into one segment. */
const MERGE_GAP_PX = 1

export interface StripSegment {
  /** The speaker whose colour the segment takes: the one who spoke longest
   *  inside it (only the shared lane can mix speakers). */
  speaker: number
  start: number
  end: number
  /** Pixel position and width in a strip `width` px wide. */
  x: number
  w: number
}

export interface StripLane {
  /** Speakers drawn in this lane: one, or several for the shared lane. */
  speakers: number[]
  segments: StripSegment[]
}

export interface StripLayout {
  lanes: StripLane[]
  /** Seconds spoken per speaker, for the screen-reader summary. */
  talkTime: Map<number, number>
  totalTalk: number
}

/** Seconds each speaker talks. One pass; malformed rows (end before start)
 *  count as zero rather than negative. */
function talkTimes(utterances: Utterance[]): Map<number, number> {
  const totals = new Map<number, number>()
  for (const utterance of utterances) {
    const length = Math.max(0, utterance.end - utterance.start)
    totals.set(utterance.speaker, (totals.get(utterance.speaker) ?? 0) + length)
  }
  return totals
}

/** Which lane each speaker gets: one each up to `MAX_LANES`; beyond that the
 *  busiest keep a lane (in speaker order, so lanes read top to bottom like the
 *  Speakers panel) and the rest fold into a last, shared lane. */
function assignLanes(totals: Map<number, number>): { laneOf: Map<number, number>; lanes: number[][] } {
  const speakers = [...totals.keys()].sort((a, b) => a - b)
  const own =
    speakers.length <= MAX_LANES
      ? speakers
      : [...speakers]
          .sort((a, b) => (totals.get(b) ?? 0) - (totals.get(a) ?? 0) || a - b)
          .slice(0, MAX_LANES - 1)
          .sort((a, b) => a - b)
  const laneOf = new Map<number, number>()
  const lanes: number[][] = own.map((speaker, index) => {
    laneOf.set(speaker, index)
    return [speaker]
  })
  const rest = speakers.filter((speaker) => !laneOf.has(speaker))
  if (rest.length > 0) {
    for (const speaker of rest) laneOf.set(speaker, lanes.length)
    lanes.push(rest)
  }
  return { laneOf, lanes }
}

/** The turn under the playhead, or undefined in a gap between turns. */
export function turnAt(utterances: Utterance[], time: number): Utterance | undefined {
  for (const utterance of utterances) {
    if (time >= utterance.start && time < utterance.end) return utterance
  }
  return undefined
}

/**
 * Where a click on a segment seeks: the time under the pointer, snapped back to
 * the start of the lane's turn at that time. A segment can stand for hundreds
 * of merged turns, so its own start says nothing about where the click landed.
 * `fraction` is the pointer's position across the segment, 0 to 1.
 */
export function seekTimeInSegment(
  utterances: Utterance[],
  speakers: number[],
  segment: Pick<StripSegment, 'start' | 'end'>,
  fraction: number,
): number {
  const target = segment.start + Math.min(1, Math.max(0, fraction)) * (segment.end - segment.start)
  let snapped = segment.start
  for (const utterance of utterances) {
    if (utterance.start <= snapped || utterance.start > target) continue
    if (utterance.end > utterance.start && speakers.includes(utterance.speaker)) snapped = utterance.start
  }
  return snapped
}

/**
 * The strip's geometry for a strip `width` px wide over `duration` seconds.
 * A turn merges with the previous segment in its lane when the gap is under a
 * pixel. In the shared lane two different speakers merge only when one of them
 * is under a pixel wide, so a long run of folded speakers keeps its colours.
 * Either way every new segment starts at least a pixel past the last one's
 * start, so a lane never holds more segments than the strip has pixels: the
 * work is bounded by the card's width, not by the turns in a two-hour hearing.
 */
export function layoutStrip(utterances: Utterance[], duration: number, width: number): StripLayout {
  const talkTime = talkTimes(utterances)
  let totalTalk = 0
  for (const seconds of talkTime.values()) totalTalk += seconds
  if (!(duration > 0) || !(width > 0) || utterances.length === 0) {
    return { lanes: [], talkTime, totalTalk }
  }

  const { laneOf, lanes: laneSpeakers } = assignLanes(talkTime)
  const raw: { speaker: number; start: number; end: number; x0: number; x1: number; lead: number }[][] =
    laneSpeakers.map(() => [])
  const scale = width / duration

  for (const utterance of utterances) {
    const length = utterance.end - utterance.start
    if (!(length > 0)) continue
    const lane = raw[laneOf.get(utterance.speaker) ?? 0]!
    const x0 = Math.min(width, Math.max(0, utterance.start * scale))
    const x1 = Math.min(width, Math.max(0, utterance.end * scale))
    const last = lane[lane.length - 1]
    const close = last !== undefined && x0 - last.x1 < MERGE_GAP_PX
    const sameSpeaker = last !== undefined && last.speaker === utterance.speaker
    const tooThin =
      last !== undefined && (last.x1 - last.x0 < MERGE_GAP_PX || x1 - x0 < MERGE_GAP_PX)
    if (last && close && (sameSpeaker || tooThin)) {
      last.x1 = Math.max(last.x1, x1)
      last.end = Math.max(last.end, utterance.end)
      // Two thin speakers merged: the colour follows whoever holds the longest
      // single stretch, which is cheap and stable.
      if (last.speaker !== utterance.speaker && length > last.lead) {
        last.speaker = utterance.speaker
        last.lead = length
      } else if (last.speaker === utterance.speaker) {
        last.lead += length
      }
      continue
    }
    lane.push({ speaker: utterance.speaker, start: utterance.start, end: utterance.end, x0, x1, lead: length })
  }

  const lanes = laneSpeakers.map((speakers, index) => ({
    speakers,
    segments: raw[index]!
      .filter((segment) => segment.x1 - segment.x0 >= MIN_SEGMENT_PX)
      .map((segment) => ({
        speaker: segment.speaker,
        start: segment.start,
        end: segment.end,
        x: segment.x0,
        w: Math.max(1, segment.x1 - segment.x0),
      })),
  }))
  return { lanes, talkTime, totalTalk }
}
