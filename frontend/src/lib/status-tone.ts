/**
 * Single status→tone map for the "quiet status language": live work pulses
 * with the accent, finished work is neutral muted (never green), and only
 * genuine failures/warnings/successes get their own color. Every status
 * render should route through this map — no per-status colors elsewhere.
 */
export type StatusTone = 'live' | 'warning' | 'error' | 'success' | 'neutral'

const LIVE_STATUSES = new Set(['pending', 'encrypting', 'submitted', 'processing', 'scan_pending'])
const NEUTRAL_STATUSES = new Set(['ready', 'completed', 'deleted', 'skipped'])
// `fail` is the audio-integrity findings' spelling of the same thing.
const ERROR_STATUSES = new Set(['failed', 'fail', 'scan_infected', 'scan_error'])
const SUCCESS_STATUSES = new Set(['scan_clean', 'pass'])
const WARNING_STATUSES = new Set(['warn'])

export function statusTone(status: string): StatusTone {
  if (LIVE_STATUSES.has(status)) return 'live'
  if (ERROR_STATUSES.has(status)) return 'error'
  if (SUCCESS_STATUSES.has(status)) return 'success'
  if (WARNING_STATUSES.has(status)) return 'warning'
  if (NEUTRAL_STATUSES.has(status)) return 'neutral'
  return 'neutral'
}

export const TONE_CLASS: Record<StatusTone, string> = {
  live: 'border-primary/40 bg-primary/10 text-primary',
  warning: 'border-warning/40 bg-warning/10 text-warning',
  error: 'border-destructive/40 bg-destructive/10 text-destructive',
  success: 'border-success/40 bg-success/10 text-success',
  neutral: 'border-border/60 bg-muted/40 text-muted-foreground',
}

/** The same tones as one solid mark, for indicators too small to carry a tinted
 *  surface plus a border — the audio-integrity findings list. */
export const TONE_DOT_CLASS: Record<StatusTone, string> = {
  live: 'bg-primary',
  warning: 'bg-warning',
  error: 'bg-destructive',
  success: 'bg-success',
  neutral: 'bg-muted-foreground',
}
