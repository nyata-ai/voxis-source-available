import { describe, it, expect, vi, afterEach } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import type { AudioPlayerHandle } from '@/components/media/AudioPlayer'
import { useReaderPlayback, type ReaderPlayback } from './useReaderPlayback'

/** The hook needs a real element on `cardRef` before its observer can attach,
 *  so these tests mount it the way the reader does and read the last value it
 *  returned. */
let latest!: ReaderPlayback

function Harness({ enabled = true }: { enabled?: boolean }) {
  latest = useReaderPlayback(enabled)
  return (
    <>
      <div ref={latest.cardRef} data-testid="card" />
      <div ref={latest.transcriptRef} data-testid="transcript" />
    </>
  )
}

function makeHandle(): AudioPlayerHandle {
  return { seek: vi.fn(), toggle: vi.fn(), setSpeed: vi.fn() }
}

/** A stand-in observer whose callback the test fires by hand. Returns how many
 *  times one was constructed, so "never observes" is testable. */
function stubIntersectionObserver() {
  const state = { constructed: 0, notify: null as ((intersecting: boolean) => void) | null }
  class FakeIntersectionObserver {
    constructor(callback: IntersectionObserverCallback) {
      state.constructed += 1
      state.notify = (isIntersecting) => {
        callback([{ isIntersecting } as IntersectionObserverEntry], this as never)
      }
    }
    observe() {}
    unobserve() {}
    disconnect() {
      state.notify = null
    }
  }
  vi.stubGlobal('IntersectionObserver', FakeIntersectionObserver)
  return state
}

describe('useReaderPlayback', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  describe('time reporting', () => {
    it('throttles the transcript highlight rather than passing every tick on', () => {
      vi.useFakeTimers()
      render(<Harness />)

      act(() => { latest.handlers.onTimeUpdate(1) })
      expect(latest.currentTime).toBe(1)

      act(() => {
        vi.advanceTimersByTime(100)
        latest.handlers.onTimeUpdate(1.1)
      })
      expect(latest.currentTime).toBe(1)

      act(() => {
        vi.advanceTimersByTime(200)
        latest.handlers.onTimeUpdate(1.3)
      })
      expect(latest.currentTime).toBe(1.3)
    })

    // A seek is not a tick. The player echoes the new position back through the
    // same callback, so a throttle left closed would swallow a transcript click
    // that landed within 250 ms of the previous report — leaving the active
    // turn, and the dock's controlled slider, on the old time.
    it('lets a seek through the throttle that would otherwise drop it', () => {
      vi.useFakeTimers()
      const handle = makeHandle()
      render(<Harness />)
      act(() => { latest.playerRef.current = handle })

      act(() => { latest.handlers.onTimeUpdate(10) })
      expect(latest.currentTime).toBe(10)

      act(() => {
        vi.advanceTimersByTime(20)
        latest.handlers.seek(120)
      })

      expect(handle.seek).toHaveBeenCalledWith(120)
      expect(latest.currentTime).toBe(120)

      // The element echoes the seek back through the same callback, still
      // inside the 250 ms window. The throttle has to be open for it, or the
      // page runs a quarter-second behind the audio from here on.
      act(() => { latest.handlers.onTimeUpdate(120.2) })
      expect(latest.currentTime).toBe(120.2)
    })

    it('moves the time even with no element to seek', () => {
      render(<Harness />)
      act(() => { latest.handlers.seek(42) })
      expect(latest.currentTime).toBe(42)
    })
  })

  describe('element state', () => {
    it('mirrors what the player reports', () => {
      render(<Harness />)

      act(() => {
        latest.handlers.onPlayStateChange(true)
        latest.handlers.onDurationChange(1796)
        latest.handlers.onSpeedChange(1.25)
      })

      expect(latest.isPlaying).toBe(true)
      expect(latest.duration).toBe(1796)
      expect(latest.speed).toBe(1.25)
    })

    it('drives the element through the handle it was given', () => {
      const handle = makeHandle()
      render(<Harness />)
      act(() => { latest.playerRef.current = handle })

      act(() => { latest.handlers.toggle() })
      act(() => { latest.handlers.setSpeed(1.5) })

      expect(handle.toggle).toHaveBeenCalledTimes(1)
      expect(handle.setSpeed).toHaveBeenCalledWith(1.5)
    })
  })

  describe('the mini dock', () => {
    // jsdom has none, and neither do the browsers that would need a polyfill.
    // A dock that showed by default would cover the page on every render.
    it('stays down without an IntersectionObserver', () => {
      render(<Harness />)
      expect(latest.dockVisible).toBe(false)
    })

    it('rises once the audio card leaves the viewport, and falls again', () => {
      const observer = stubIntersectionObserver()
      render(<Harness />)
      expect(latest.dockVisible).toBe(false)

      act(() => { observer.notify?.(false) })
      expect(latest.dockVisible).toBe(true)

      act(() => { observer.notify?.(true) })
      expect(latest.dockVisible).toBe(false)
    })

    it('observes nothing at all when there is no player on the page', () => {
      const observer = stubIntersectionObserver()
      render(<Harness enabled={false} />)
      expect(observer.constructed).toBe(0)
      expect(latest.dockVisible).toBe(false)
    })

    // The transcript and not the audio card: below `lg` the card sits above the
    // words, so scrolling to it lands short of what the label promises.
    it('sends the reader back to the transcript, not the audio card', () => {
      render(<Harness />)
      const card = screen.getByTestId('card')
      const transcript = screen.getByTestId('transcript')
      card.scrollIntoView = vi.fn()
      transcript.scrollIntoView = vi.fn()

      act(() => { latest.handlers.backToTranscript() })

      expect(transcript.scrollIntoView).toHaveBeenCalledWith({ block: 'start', behavior: 'smooth' })
      expect(card.scrollIntoView).not.toHaveBeenCalled()
    })
  })
})
