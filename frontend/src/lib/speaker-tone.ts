/**
 * Speaker identity colours (DESIGN.md, Speaker Palette Rule). Keyed on the
 * provider's speaker number, never on the name, so renaming a speaker keeps
 * their colour. The colours only ever fill a disc or a strip segment; the
 * initial and the name beside them carry the meaning, so nothing depends on
 * colour alone.
 */

/** Fill utilities in palette order: peach, mint, lilac, butter, sky. Literal
 *  class names so Tailwind generates them. */
const SPEAKER_FILLS = ['bg-peach', 'bg-mint', 'bg-lilac', 'bg-butter', 'bg-sky'] as const

export const SPEAKER_PALETTE_SIZE = SPEAKER_FILLS.length

export interface SpeakerTone {
  /** Background utility for the disc or strip segment. */
  fill: string
  /** True from the second pass through the palette (speaker 6 on), drawn with a
   *  1.5px ink ring so speaker 1 and speaker 6 still differ. Past speaker 10 a
   *  fill and ring repeat; the name and initial still tell them apart. */
  ringed: boolean
}

/**
 * The tone for a zero-based provider speaker number. Anything that is not a
 * non-negative integer (a malformed row) falls back to the first tone rather
 * than throwing inside a render.
 */
export function speakerTone(speakerIndex: number): SpeakerTone {
  const index = Number.isInteger(speakerIndex) && speakerIndex >= 0 ? speakerIndex : 0
  const pass = Math.floor(index / SPEAKER_PALETTE_SIZE)
  return {
    fill: SPEAKER_FILLS[index % SPEAKER_PALETTE_SIZE]!,
    ringed: pass >= 1,
  }
}

/**
 * What a speaker disc shows: the first character of the name (a whole
 * character, so a CJK name or an emoji is not split), upper-cased where the
 * script has case; the one-based speaker number while the speaker is unnamed.
 */
export function speakerInitial(name: string | undefined, speakerIndex: number): string {
  const first = Array.from(name?.trim() ?? '')[0]
  if (first) return first.toLocaleUpperCase()
  const index = Number.isInteger(speakerIndex) && speakerIndex >= 0 ? speakerIndex : 0
  return String(index + 1)
}
