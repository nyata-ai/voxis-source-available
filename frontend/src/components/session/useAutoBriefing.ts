import { useEffect, useRef, useState } from 'react'
import type { SessionLens } from './session-view'

/** The guard key for one session. Session-scoped storage on purpose: the point
 *  is "not twice in this visit", not "never again" — a later visit that still
 *  finds no briefings should be free to try. */
export function autoBriefingKey(sessionId: string): string {
  return `voxis.autobrief.${sessionId}`
}

function hasAlreadyFired(sessionId: string): boolean {
  try {
    return sessionStorage.getItem(autoBriefingKey(sessionId)) !== null
  } catch {
    // Storage unreadable — fall through to the in-mount guard. Firing once more
    // costs nothing: both generation paths only fill lenses that are missing.
    return false
  }
}

function markFired(sessionId: string): void {
  try {
    sessionStorage.setItem(autoBriefingKey(sessionId), '1')
  } catch {
    // Storage unwritable (Safari private mode, blocked cookies). The in-mount
    // guard still holds for this mount; a remount may try once more.
  }
}

/**
 * Retracts the guard for one session, so a later visit may auto-generate again.
 *
 * The guard is written *before* `generate` runs, which is right for every
 * failure that a retry would not fix — but wrong for a 429, where the only
 * thing missing is time. Without this, one unlucky fan-out against the rate
 * limiter left the session with four empty lenses and no automatic recovery for
 * the rest of the browsing session.
 *
 * Deliberately only the persisted half: the caller's mount keeps its in-memory
 * ref, so clearing this cannot start a retry loop against the very limiter that
 * refused. The retry happens on the next visit to the session — by then the
 * tokens have refilled — and the pane's Generate button remains the immediate
 * manual path.
 */
export function clearAutoBriefingGuard(sessionId: string): void {
  if (!sessionId) return
  try {
    sessionStorage.removeItem(autoBriefingKey(sessionId))
  } catch {
    // Storage unavailable. Nothing was persisted either, so there is nothing
    // holding the next visit back.
  }
}

export interface UseAutoBriefingOptions {
  /** Transcription id — the guard is scoped to it. */
  sessionId: string
  status: string
  lenses: SessionLens[]
  /**
   * Whether the summaries read **succeeded**. Not `!isLoading`: a failed read
   * and a session with no briefings both arrive as an empty lens set, and only
   * a successful read is evidence that there is nothing to open. Passing the
   * inverse of a loading flag would generate a second set of briefings every
   * time the list request failed.
   */
  summariesReady: boolean
  /** Fills every missing lens. Called at most once per session per visit. */
  generate: () => void
  /**
   * The whole gate, in one flag. False when the session does not permit
   * briefings **or** when the reader has not opted in: automatic generation is
   * the `auto_briefings` preference, off by default, and the adapters fold it in
   * here rather than adding a second flag the hook would have to AND together.
   * Until it is true this hook does nothing at all — no guard is written, so
   * turning the preference on later still gets its one automatic run.
   */
  enabled: boolean
}

export interface AutoBriefingState {
  /** True when this visit started the generation **for the session currently
   *  being read**, so the pane can say so instead of appearing to work on its
   *  own. Resets the moment the reader moves to a different session. */
  autoStarted: boolean
}

/**
 * Opens a finished session's four briefing lenses without being asked, once —
 * and only for a reader who turned `auto_briefings` on. The default is off:
 * generating four briefings is billable AI work, and most sessions want one or
 * two of them, chosen from the pane.
 *
 * The precondition is narrow by design — the opt-in, a completed session, a
 * summaries read that actually succeeded, and not one lens generated — because every failure
 * mode here is a duplicate fan-out against a rate-limited, billable endpoint.
 * The guard is written *before* `generate` runs, so a rejected attempt ends the
 * matter for the visit; the pane's Generate button is the retry. The one
 * exception is a 429, which the adapters retract with
 * `clearAutoBriefingGuard` — see there for why that cannot loop.
 */
export function useAutoBriefing({
  sessionId,
  status,
  lenses,
  summariesReady,
  generate,
  enabled,
}: UseAutoBriefingOptions): AutoBriefingState {
  // Which session this mount has already fired for — not *whether* it has.
  // `/transcriptions/:id` carries no `key`, so React Router keeps this
  // component mounted when only the param changes, and the reader navigates to
  // a new id from inside itself (the re-transcribe dialog opens the fresh
  // transcription). A boolean here would let the previous session's guard
  // silence the new one for good.
  const firedForRef = useRef<string | null>(null)
  const [startedFor, setStartedFor] = useState<string | null>(null)

  const hasAnyBriefing = lenses.some((lens) => Boolean(lens.summaryId))

  useEffect(() => {
    if (firedForRef.current === sessionId) return
    if (!enabled || !sessionId) return
    if (status !== 'completed') return
    if (!summariesReady || hasAnyBriefing) return
    if (hasAlreadyFired(sessionId)) return

    firedForRef.current = sessionId
    markFired(sessionId)
    setStartedFor(sessionId)
    generate()
  }, [enabled, sessionId, status, summariesReady, hasAnyBriefing, generate])

  // Derived rather than reset: a session the reader has moved away from must
  // not keep claiming credit for work in a pane that is now showing someone
  // else's lenses.
  return { autoStarted: startedFor === sessionId }
}
