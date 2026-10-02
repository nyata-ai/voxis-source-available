import { useCallback, useEffect, useRef, useState } from 'react'

/** Stream URLs are short-lived and signed; a stale one fails once and a refetch
 *  usually fixes it. Three refetches, then the audio is reported as failed. */
const MAX_STREAM_RETRIES = 3

/** What an adapter hands to its audio rail. `onLoaded` and `retry` are optional
 *  on `SessionAudio` because a source with no player never produces them; this
 *  hook always does. */
export interface StreamRetry {
  streamFailed: boolean
  onStreamError: () => void
  onStreamLoaded: () => void
  retryStream: () => void
}

/**
 * The reader's stream retry, shared by every adapter that docks a player: an
 * `<audio>` error refetches the signed URL up to `MAX_STREAM_RETRIES` times and
 * then gives up, which is what turns the audio rail's state to `failed`. The
 * counter resets when `id` changes, so navigating to another session starts
 * over rather than inheriting the previous one's exhausted budget.
 *
 * It also resets on a *successful* load (`onStreamLoaded`). Without that the
 * budget only ever spends down: a session played for an hour, dropping one
 * stale URL every twenty minutes, would exhaust three retries and report failed
 * audio that had in fact recovered every time.
 *
 * `retryStream` is the manual way back once the budget is gone — the audio
 * rail's retry button — because the automatic path has, by then, given up.
 */
export function useStreamRetry(id: string, refetchStream: () => void): StreamRetry {
  const [streamFailed, setStreamFailed] = useState(false)
  const retries = useRef(0)

  useEffect(() => {
    retries.current = 0
    setStreamFailed(false)
  }, [id])

  const onStreamError = useCallback(() => {
    // Counted first, compared with `>`: the third error still gets its refetch,
    // and only a failure *after* the last retry has been spent is terminal.
    retries.current += 1
    if (retries.current > MAX_STREAM_RETRIES) {
      setStreamFailed(true)
      return
    }
    refetchStream()
  }, [refetchStream])

  const onStreamLoaded = useCallback(() => {
    // Guarded so the ordinary case — a load with nothing to forgive — does not
    // call setState on every `loadedmetadata` the player emits.
    if (retries.current === 0) return
    retries.current = 0
    setStreamFailed(false)
  }, [])

  const retryStream = useCallback(() => {
    retries.current = 0
    setStreamFailed(false)
    refetchStream()
  }, [refetchStream])

  return { streamFailed, onStreamError, onStreamLoaded, retryStream }
}
