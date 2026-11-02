import type { TFunction } from 'i18next'
import type { ActivityItem, ActivityStage } from '@/types/activity'
import type { StepState } from './StepTrack'

const STAGE_KEYS: Record<ActivityStage, string> = {
  stitching: 'activity.stage.stitching',
  transcribing: 'activity.stage.transcribing',
  summarizing: 'activity.stage.summarizing',
  ready: 'activity.stage.ready',
}

export function activityStageKey(stage: ActivityStage): string {
  return STAGE_KEYS[stage]
}

/**
 * The one line of text a job shows: the step it is on, "Ready", or what
 * failed. Shared by the tray card, the Desk band and their live announcements
 * so a job never reads two different ways on one screen.
 */
export function activityLabel(item: ActivityItem, t: TFunction): string {
  if (item.status === 'failed') return t(`activity.failed.${item.kind}`)
  if (item.status === 'completed') return t('activity.ready')
  return t(activityStageKey(item.stage), {
    terminal: item.detail?.terminal ?? 0,
    total: item.detail?.total ?? 0,
  })
}

/**
 * The step track for a job, drawing only the steps this job really has.
 *  - A recording: Saving → Ready (the stitched file is then a media item).
 *  - A transcription: Transcribing → (Briefing) → Ready. Briefings are drawn
 *    only once the server reports them running (`detail`), because nothing
 *    says in advance that a deployment or a session will produce any.
 */
export function activitySteps(item: ActivityItem): StepState[] {
  const hasBriefing = item.kind === 'transcription' && item.stage === 'summarizing'
  const count = hasBriefing ? 3 : 2
  if (item.status === 'completed') return Array<StepState>(count).fill('done')
  // Only the transcription step can fail: a briefing failure leaves the
  // transcript itself ready, and the server reports it as completed.
  if (item.status === 'failed') return ['failed', ...Array<StepState>(count - 1).fill('pending')]
  if (hasBriefing) return ['done', 'current', 'pending']
  return ['current', 'pending']
}

export type Intake = 'record' | 'upload' | 'link'

/**
 * Which way the work came in, for the intake disc. A transcription made from a
 * live recording cannot yet be told apart from an upload (both point at a
 * media file), so it reads as an upload until the feed says otherwise.
 */
export function activityIntake(item: ActivityItem): Intake {
  if (item.kind === 'recording') return 'record'
  if (item.link?.startsWith('/url-transcriptions/')) return 'link'
  return 'upload'
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
