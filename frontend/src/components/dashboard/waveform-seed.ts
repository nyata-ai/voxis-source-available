/**
 * Deterministic waveform-bar generator.
 *
 * Same seed in → same bar pattern out, so every transcript has a stable visual
 * signature without storing real audio peaks. The output is "voice-shaped":
 * a smoothed noise sequence tapered by a sine envelope (quiet at the edges,
 * fuller in the middle) so it always reads as a spoken-word waveform.
 *
 * Hash: FNV-1a (32-bit) → PRNG: mulberry32.
 */

// Smoothing weight blend between successive noise samples. Higher values for
// `PREV_WEIGHT` produce smoother, more correlated bars (voice-like persistence).
const PREV_WEIGHT = 0.45
const NEXT_WEIGHT = 0.55

// Bars are scaled into [MIN_AMPLITUDE, 1] after the envelope multiply, so even
// the quietest edge bars remain visually present rather than disappearing.
const MIN_AMPLITUDE_HERO = 0.08
const MIN_AMPLITUDE_MINI = 0.15

// Floor + envelope gain. amplitude = FLOOR + GAIN * sin(πt) * smoothedNoise.
const HERO_FLOOR = 0.15
const HERO_GAIN = 0.85
const MINI_FLOOR = 0.25
const MINI_GAIN = 0.75

/** FNV-1a 32-bit hash for string seeds. Stable, fast, non-cryptographic. */
export function hashString(value: string): number {
  let h = 2166136261
  for (let i = 0; i < value.length; i++) {
    h ^= value.charCodeAt(i)
    h = Math.imul(h, 16777619) >>> 0
  }
  return h >>> 0
}

/** Mulberry32 PRNG. Returns a function that yields uniform values in [0, 1). */
export function mulberry32(seed: number): () => number {
  let state = seed >>> 0
  return () => {
    state = (state + 0x6d2b79f5) >>> 0
    let t = state
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

interface GenerateOptions {
  /** Variant tuning — hero bars have a deeper noise floor and louder gain. */
  shape: 'hero' | 'mini'
  /** Optional extra entropy mixed into the seed (e.g. speaker count, duration). */
  saltA?: number
  saltB?: number
}

export function generateBars(
  seed: string,
  count: number,
  options: GenerateOptions
): number[] {
  const { shape, saltA = 0, saltB = 0 } = options
  const seedKey = `${seed}|a${saltA}|b${Math.floor(saltB)}`
  const rng = mulberry32(hashString(seedKey))

  const floor = shape === 'hero' ? HERO_FLOOR : MINI_FLOOR
  const gain = shape === 'hero' ? HERO_GAIN : MINI_GAIN
  const minAmp = shape === 'hero' ? MIN_AMPLITUDE_HERO : MIN_AMPLITUDE_MINI

  const out: number[] = []
  let prev = rng()
  for (let i = 0; i < count; i++) {
    const next = rng()
    const smoothed = prev * PREV_WEIGHT + next * NEXT_WEIGHT
    prev = smoothed

    const t = count <= 1 ? 0.5 : i / (count - 1)
    const envelope = Math.sin(Math.PI * t)
    const amp = floor + gain * envelope * smoothed
    out.push(Math.max(minAmp, Math.min(1, amp)))
  }
  return out
}
