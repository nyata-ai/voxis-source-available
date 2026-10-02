import { vi } from 'vitest'

/**
 * Run fn with Intl reporting the given timezone, or throwing when zone is null.
 *
 * Shared by geo-language.test.ts and detectors.test.ts, which are the two sides
 * of the same rule: the zone table and the detector that yields to it. They held
 * separate copies that had drifted apart — only one could simulate an Intl that
 * throws, which is the branch locked-down browsers actually take.
 *
 * The whole Intl.DateTimeFormat constructor is replaced rather than the process
 * timezone, because vitest.config.ts pins TZ=UTC for the suite and Node resolves
 * that once, at startup.
 */
export function withTimeZone(zone: string | null, fn: () => void) {
  const spy = vi.spyOn(Intl, 'DateTimeFormat').mockImplementation((() => {
    if (zone === null) throw new RangeError('Intl unavailable')
    return { resolvedOptions: () => ({ timeZone: zone }) }
  }) as unknown as typeof Intl.DateTimeFormat)
  try {
    fn()
  } finally {
    spy.mockRestore()
  }
}
