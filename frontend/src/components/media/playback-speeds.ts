/**
 * The playback rungs every transport offers. Shared by the audio player and
 * the reader's mini dock so the two controls can never present different
 * choices for the same element.
 */
export const SPEED_OPTIONS: readonly number[] = [0.75, 1, 1.25, 1.5, 2]

/**
 * The rungs to show for a current rate. A stored preference that lands between
 * two of them is added as an extra rung rather than snapped, so the control
 * never displays a speed the audio is not actually playing at.
 */
export function speedRungs(current: number): number[] {
  if (!Number.isFinite(current) || current <= 0) return [...SPEED_OPTIONS]
  if (SPEED_OPTIONS.includes(current)) return [...SPEED_OPTIONS]
  return [...SPEED_OPTIONS, current].sort((a, b) => a - b)
}
