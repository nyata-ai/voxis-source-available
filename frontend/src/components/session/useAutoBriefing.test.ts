import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import {
  clearAutoBriefingGuard,
  useAutoBriefing,
  autoBriefingKey,
  type UseAutoBriefingOptions,
} from './useAutoBriefing'
import type { SessionLens } from './session-view'

const EMPTY_LENSES: SessionLens[] = [
  { type: 'general' },
  { type: 'key_points' },
  { type: 'action_items' },
  { type: 'q_and_a' },
]

function withOneLens(): SessionLens[] {
  return [
    { type: 'general', summaryId: 'sum-1', status: 'completed' },
    { type: 'key_points' },
    { type: 'action_items' },
    { type: 'q_and_a' },
  ]
}

function options(overrides: Partial<UseAutoBriefingOptions> = {}): UseAutoBriefingOptions {
  return {
    sessionId: 'tx-1',
    status: 'completed',
    lenses: EMPTY_LENSES,
    summariesReady: true,
    generate: vi.fn(),
    enabled: true,
    ...overrides,
  }
}

describe('useAutoBriefing', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('generates once when a completed session has no briefings at all', () => {
    const generate = vi.fn()
    const { rerender } = renderHook((props: UseAutoBriefingOptions) => useAutoBriefing(props), {
      initialProps: options({ generate }),
    })

    expect(generate).toHaveBeenCalledTimes(1)

    // Adapters rebuild `lenses` and `generate` on every render; re-running the
    // effect must not re-run the fan-out.
    rerender(options({ generate }))
    rerender(options({ generate }))
    expect(generate).toHaveBeenCalledTimes(1)
  })

  it('reports that this visit started the generation', () => {
    const { result } = renderHook(() => useAutoBriefing(options()))
    expect(result.current.autoStarted).toBe(true)
  })

  it('reports nothing when it did not start anything', () => {
    const { result } = renderHook(() => useAutoBriefing(options({ lenses: withOneLens() })))
    expect(result.current.autoStarted).toBe(false)
  })

  it('waits while the summaries are still loading', () => {
    const generate = vi.fn()
    renderHook(() => useAutoBriefing(options({ generate, summariesReady: false })))
    expect(generate).not.toHaveBeenCalled()
  })

  // A failed summaries read is indistinguishable from "this session has no
  // briefings": both arrive as an empty lens set. Only a *successful* read is
  // evidence that there is nothing there, so the caller passes the query's
  // success — never the inverse of its loading flag.
  it('does not treat a failed summaries read as an empty session', () => {
    const generate = vi.fn()
    const { rerender } = renderHook((props: UseAutoBriefingOptions) => useAutoBriefing(props), {
      initialProps: options({ generate, summariesReady: false }),
    })
    // The query settles into an error: still not ready, still nothing fired.
    rerender(options({ generate, summariesReady: false }))
    expect(generate).not.toHaveBeenCalled()
  })

  it('leaves a session that already has one briefing alone', () => {
    const generate = vi.fn()
    renderHook(() => useAutoBriefing(options({ generate, lenses: withOneLens() })))
    expect(generate).not.toHaveBeenCalled()
  })

  it('does nothing for a session that has not finished transcribing', () => {
    const generate = vi.fn()
    renderHook(() => useAutoBriefing(options({ generate, status: 'processing' })))
    expect(generate).not.toHaveBeenCalled()
  })

  // `enabled` carries both gates: the session permitting briefings at all, and
  // the reader's `auto_briefings` opt-in, which is off by default.
  it('does nothing when the session does not permit briefings, or the reader opted out', () => {
    const generate = vi.fn()
    renderHook(() => useAutoBriefing(options({ generate, enabled: false })))
    expect(generate).not.toHaveBeenCalled()
  })

  // Writing the guard while disabled would spend the session's one automatic
  // run on a visit that never generated anything, so turning the preference on
  // mid-visit would look like it did nothing.
  it('writes no guard while disabled, so enabling it later still fires', () => {
    const generate = vi.fn()
    const { rerender } = renderHook((props: UseAutoBriefingOptions) => useAutoBriefing(props), {
      initialProps: options({ generate, enabled: false }),
    })
    expect(sessionStorage.getItem(autoBriefingKey('tx-1'))).toBeNull()

    rerender(options({ generate, enabled: true }))
    expect(generate).toHaveBeenCalledTimes(1)
  })

  it('does nothing without a session id', () => {
    const generate = vi.fn()
    renderHook(() => useAutoBriefing(options({ generate, sessionId: '' })))
    expect(generate).not.toHaveBeenCalled()
  })

  it('stays quiet on a remount once the guard is set', () => {
    const generate = vi.fn()
    const first = renderHook(() => useAutoBriefing(options({ generate })))
    expect(generate).toHaveBeenCalledTimes(1)
    first.unmount()

    const second = renderHook(() => useAutoBriefing(options({ generate })))
    expect(generate).toHaveBeenCalledTimes(1)
    expect(second.result.current.autoStarted).toBe(false)
  })

  // `/transcriptions/:id` carries no `key`, so React Router keeps the route
  // component mounted when only the param changes — and the reader navigates to
  // a new id from inside itself, via the re-transcribe dialog. A guard that
  // only remembered *whether* it had fired would silence the fresh session for
  // good and leave the old session's "started automatically" line beside four
  // lenses that were never generated.
  it('fires again when the same mount moves to another session', () => {
    const generate = vi.fn()
    const { result, rerender } = renderHook(
      (props: UseAutoBriefingOptions) => useAutoBriefing(props),
      { initialProps: options({ generate }) },
    )
    expect(generate).toHaveBeenCalledTimes(1)
    expect(result.current.autoStarted).toBe(true)

    rerender(options({ generate, sessionId: 'tx-2' }))

    expect(generate).toHaveBeenCalledTimes(2)
    expect(sessionStorage.getItem(autoBriefingKey('tx-2'))).not.toBeNull()
    expect(result.current.autoStarted).toBe(true)

    // And back again: tx-1's persisted guard still holds, so nothing re-fires.
    rerender(options({ generate }))
    expect(generate).toHaveBeenCalledTimes(2)
  })

  it('consults the new session own guard rather than the previous one', () => {
    const generate = vi.fn()
    sessionStorage.setItem(autoBriefingKey('tx-2'), '1')
    const { result, rerender } = renderHook(
      (props: UseAutoBriefingOptions) => useAutoBriefing(props),
      { initialProps: options({ generate }) },
    )
    expect(generate).toHaveBeenCalledTimes(1)

    rerender(options({ generate, sessionId: 'tx-2' }))

    expect(generate).toHaveBeenCalledTimes(1)
    // The previous session started generation; this one did not, and must not
    // inherit the claim.
    expect(result.current.autoStarted).toBe(false)
  })

  it('guards per session, not globally', () => {
    const generate = vi.fn()
    renderHook(() => useAutoBriefing(options({ generate })))
    renderHook(() => useAutoBriefing(options({ generate, sessionId: 'tx-2' })))
    expect(generate).toHaveBeenCalledTimes(2)
    expect(sessionStorage.getItem(autoBriefingKey('tx-1'))).not.toBeNull()
    expect(sessionStorage.getItem(autoBriefingKey('tx-2'))).not.toBeNull()
  })

  // The guard has to be written before the call, not after it: a generation
  // that fails — a 429 from the billable limiter, a 503 from an unconfigured
  // provider — re-renders the adapter with the lenses still empty, and a guard
  // written afterwards would never have been reached.
  it('marks the session before it calls generate', () => {
    let keyAtCallTime: string | null = null
    const generate = vi.fn(() => {
      keyAtCallTime = sessionStorage.getItem(autoBriefingKey('tx-1'))
    })

    renderHook(() => useAutoBriefing(options({ generate })))

    expect(generate).toHaveBeenCalledTimes(1)
    expect(keyAtCallTime).not.toBeNull()
  })

  // The failure itself is terminal for the visit: the pane's Generate button is
  // the manual retry, and nothing re-arms the automatic one.
  it('does not retry after a failed generation', () => {
    const generate = vi.fn()
    const { rerender } = renderHook((props: UseAutoBriefingOptions) => useAutoBriefing(props), {
      initialProps: options({ generate }),
    })

    // The fan-out rejected: lenses are still empty and the query still succeeds.
    rerender(options({ generate }))
    expect(generate).toHaveBeenCalledTimes(1)
  })

  // Safari private mode and blocked cookies make sessionStorage throw. The
  // persisted guard is then unavailable, so an in-mount guard has to carry it.
  it('still fires only once when sessionStorage cannot be written', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('QuotaExceededError')
    })
    const generate = vi.fn()
    const { rerender } = renderHook((props: UseAutoBriefingOptions) => useAutoBriefing(props), {
      initialProps: options({ generate }),
    })

    rerender(options({ generate }))
    rerender(options({ generate }))
    expect(generate).toHaveBeenCalledTimes(1)
  })

  it('still fires when sessionStorage cannot be read', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError')
    })
    const generate = vi.fn()
    renderHook(() => useAutoBriefing(options({ generate })))
    expect(generate).toHaveBeenCalledTimes(1)
  })
})

describe('clearAutoBriefingGuard', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  // The whole point: a 429 leaves the session with four empty lenses and a
  // guard that would have silenced the automatic retry for the rest of the
  // browsing session. Retracting the guard lets the next visit try again.
  it('lets a later visit auto-generate again', () => {
    const generate = vi.fn()
    const first = renderHook(() => useAutoBriefing(options({ generate })))
    expect(generate).toHaveBeenCalledTimes(1)
    first.unmount()

    clearAutoBriefingGuard('tx-1')

    renderHook(() => useAutoBriefing(options({ generate })))
    expect(generate).toHaveBeenCalledTimes(2)
  })

  // Clearing must not re-arm the *current* mount: that would fire straight back
  // into the limiter that just refused. The in-memory ref is deliberately left
  // alone, so recovery waits for a fresh visit.
  it('does not re-fire inside the mount that cleared it', () => {
    const generate = vi.fn()
    const { rerender } = renderHook((props: UseAutoBriefingOptions) => useAutoBriefing(props), {
      initialProps: options({ generate }),
    })
    expect(generate).toHaveBeenCalledTimes(1)

    clearAutoBriefingGuard('tx-1')
    rerender(options({ generate }))
    rerender(options({ generate }))

    expect(generate).toHaveBeenCalledTimes(1)
  })

  it('clears only the session it names', () => {
    sessionStorage.setItem(autoBriefingKey('tx-1'), '1')
    sessionStorage.setItem(autoBriefingKey('tx-2'), '1')

    clearAutoBriefingGuard('tx-1')

    expect(sessionStorage.getItem(autoBriefingKey('tx-1'))).toBeNull()
    expect(sessionStorage.getItem(autoBriefingKey('tx-2'))).not.toBeNull()
  })

  it('ignores an empty session id rather than touching a stray key', () => {
    sessionStorage.setItem(autoBriefingKey(''), '1')
    clearAutoBriefingGuard('')
    expect(sessionStorage.getItem(autoBriefingKey(''))).not.toBeNull()
  })

  it('survives storage that throws', () => {
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => {
      throw new Error('SecurityError')
    })
    expect(() => clearAutoBriefingGuard('tx-1')).not.toThrow()
  })
})
