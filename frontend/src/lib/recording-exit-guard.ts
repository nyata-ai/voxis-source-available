import type { Location } from 'react-router-dom'
import { useRecordingStore } from '@/stores/recording'

/**
 * React Router blocker predicate for the recording screen.
 *
 * Reads live store state (not a render snapshot) so it stays correct no matter
 * when the router re-registers the blocker. `completing` is guarded too: the
 * server session stays `recording` until the complete call returns, so leaving
 * mid-drain strands it with its audio unreachable.
 */
export function shouldBlockRecordingExit({
  currentLocation,
  nextLocation,
}: {
  currentLocation: Location
  nextLocation: Location
}): boolean {
  if (currentLocation.pathname === nextLocation.pathname) return false
  const status = useRecordingStore.getState().status
  return status === 'recording' || status === 'paused' || status === 'completing'
}

export function isHeartbeatCadenceSafe(
  heartbeatIntervalMs: number,
  orphanThresholdMinutes: number
): boolean {
  if (heartbeatIntervalMs <= 0 || orphanThresholdMinutes <= 0) {
    return false
  }
  const orphanThresholdMs = orphanThresholdMinutes * 60 * 1000
  return heartbeatIntervalMs <= orphanThresholdMs / 3
}
