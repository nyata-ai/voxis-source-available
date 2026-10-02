import type { ActivityStage, ActivityStatus } from '@/types/activity'

export type ActivityTone = 'spinner' | 'success' | 'error'

const STAGE_KEYS: Record<ActivityStage, string> = {
  stitching: 'activity.stage.stitching',
  transcribing: 'activity.stage.transcribing',
  summarizing: 'activity.stage.summarizing',
  ready: 'activity.stage.ready',
}

const STATUS_TONES: Record<ActivityStatus, ActivityTone> = {
  in_progress: 'spinner',
  completed: 'success',
  failed: 'error',
}

export function activityStageKey(stage: ActivityStage): string {
  return STAGE_KEYS[stage]
}

export function activityStatusTone(status: ActivityStatus): ActivityTone {
  return STATUS_TONES[status]
}

/**
 * Coarse "how long has this been running" label — seconds, then minutes, then
 * hours, never mixed. Shared by the floating tray card and the Desk's inline
 * band so one item never reads two different ages on the same screen.
 * Returns '' for an unparseable timestamp, which callers omit entirely.
 */
export function elapsedLabel(startedAt: string, now: number): string {
  const started = new Date(startedAt).getTime()
  if (Number.isNaN(started)) return ''
  const elapsedSeconds = Math.max(0, Math.floor((now - started) / 1000))
  if (elapsedSeconds < 60) return `${elapsedSeconds}s`
  const elapsedMinutes = Math.floor(elapsedSeconds / 60)
  if (elapsedMinutes < 60) return `${elapsedMinutes}m`
  return `${Math.floor(elapsedMinutes / 60)}h`
}
