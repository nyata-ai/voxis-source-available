import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useTranscriptAutoScroll } from './useTranscriptAutoScroll'

function createMockContainer(): HTMLDivElement {
  const div = document.createElement('div')
  Object.defineProperty(div, 'scrollHeight', { value: 2000, configurable: true })
  Object.defineProperty(div, 'clientHeight', { value: 500, configurable: true })
  Object.defineProperty(div, 'scrollTop', { value: 0, writable: true, configurable: true })
  Object.defineProperty(div, 'offsetTop', { value: 0, configurable: true })
  div.scrollTo = vi.fn()
  return div
}

function createMockElement(offsetTop = 800, offsetHeight = 40): HTMLDivElement {
  const el = document.createElement('div')
  Object.defineProperty(el, 'offsetTop', { value: offsetTop, configurable: true })
  Object.defineProperty(el, 'offsetHeight', { value: offsetHeight, configurable: true })
  return el
}

describe('useTranscriptAutoScroll', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('scrolls to active segment when activeSegmentId changes while following', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const mockElement = createMockElement()

    const { result, rerender } = renderHook(
      ({ activeId }) =>
        useTranscriptAutoScroll({
          containerRef,
          activeSegmentId: activeId,
          isFollowing: true,
          onFollowingChange: vi.fn(),
        }),
      { initialProps: { activeId: null as number | null } },
    )

    act(() => {
      result.current.registerSegmentRef(1, mockElement)
    })

    rerender({ activeId: 1 })

    expect(container.scrollTo).toHaveBeenCalledWith({
      top: expect.any(Number),
      behavior: 'smooth',
    })
  })

  it('does not scroll when isFollowing is false', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const mockElement = createMockElement()

    const { result, rerender } = renderHook(
      ({ activeId }) =>
        useTranscriptAutoScroll({
          containerRef,
          activeSegmentId: activeId,
          isFollowing: false,
          onFollowingChange: vi.fn(),
        }),
      { initialProps: { activeId: null as number | null } },
    )

    act(() => {
      result.current.registerSegmentRef(1, mockElement)
    })

    rerender({ activeId: 1 })
    expect(container.scrollTo).not.toHaveBeenCalled()
  })

  // The hook no longer owns follow state: a manual scroll is reported upward,
  // and the owner decides. Reporting instead of setting is what lets the
  // toolbar switch and the in-view pill agree on one value.
  it('reports a release upward when the reader scrolls away', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const onFollowingChange = vi.fn()

    renderHook(() =>
      useTranscriptAutoScroll({
        containerRef,
        activeSegmentId: null,
        isFollowing: true,
        onFollowingChange,
      }),
    )

    act(() => {
      container.scrollTop = 100
      container.dispatchEvent(new Event('scroll'))
    })

    expect(onFollowingChange).toHaveBeenCalledWith(false)
  })

  it('does not re-report a release while already released', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const onFollowingChange = vi.fn()

    renderHook(() =>
      useTranscriptAutoScroll({
        containerRef,
        activeSegmentId: null,
        isFollowing: false,
        onFollowingChange,
      }),
    )

    act(() => {
      container.scrollTop = 100
      container.dispatchEvent(new Event('scroll'))
    })

    expect(onFollowingChange).not.toHaveBeenCalled()
  })

  it('ignores small scroll movements below threshold', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const onFollowingChange = vi.fn()

    renderHook(() =>
      useTranscriptAutoScroll({
        containerRef,
        activeSegmentId: null,
        isFollowing: true,
        onFollowingChange,
      }),
    )

    act(() => {
      container.scrollTop = 10
      container.dispatchEvent(new Event('scroll'))
    })

    expect(onFollowingChange).not.toHaveBeenCalled()
  })

  it('followPlayback reports true and scrolls to the active segment', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const mockElement = createMockElement()
    const onFollowingChange = vi.fn()

    const { result } = renderHook(() =>
      useTranscriptAutoScroll({
        containerRef,
        activeSegmentId: 1,
        isFollowing: false,
        onFollowingChange,
      }),
    )

    act(() => {
      result.current.registerSegmentRef(1, mockElement)
    })

    act(() => {
      result.current.followPlayback()
    })

    expect(onFollowingChange).toHaveBeenCalledWith(true)
    expect(container.scrollTo).toHaveBeenCalledWith({
      top: expect.any(Number),
      behavior: 'smooth',
    })
  })

  it('jumpToSegment scrolls to the segment and reports a release', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const mockElement = createMockElement()
    const onFollowingChange = vi.fn()

    const { result } = renderHook(() =>
      useTranscriptAutoScroll({
        containerRef,
        activeSegmentId: 0,
        isFollowing: true,
        onFollowingChange,
      }),
    )

    act(() => {
      result.current.registerSegmentRef(7, mockElement)
    })

    act(() => {
      result.current.jumpToSegment(7)
    })

    expect(container.scrollTo).toHaveBeenCalledWith({
      top: expect.any(Number),
      behavior: 'smooth',
    })
    // Following must stop, or the next playback tick yanks the view back off
    // the match the reader just jumped to.
    expect(onFollowingChange).toHaveBeenCalledWith(false)
  })

  it('jumpToSegment does not scroll for an unregistered segment', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const onFollowingChange = vi.fn()

    const { result } = renderHook(() =>
      useTranscriptAutoScroll({
        containerRef,
        activeSegmentId: null,
        isFollowing: true,
        onFollowingChange,
      }),
    )

    act(() => {
      result.current.jumpToSegment(42)
    })

    expect(container.scrollTo).not.toHaveBeenCalled()
    expect(onFollowingChange).toHaveBeenCalledWith(false)
  })

  it('cleans up scroll listener on unmount', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const removeSpy = vi.spyOn(container, 'removeEventListener')

    const { unmount } = renderHook(() =>
      useTranscriptAutoScroll({
        containerRef,
        activeSegmentId: null,
        isFollowing: true,
        onFollowingChange: vi.fn(),
      }),
    )

    unmount()
    expect(removeSpy).toHaveBeenCalledWith('scroll', expect.any(Function))
  })

  // A viewer that mounts with no utterances renders no scroll box, so the ref is
  // still null on the first effect pass. The ref object never changes identity,
  // so without a second key the listener would be subscribed once — against
  // nothing — and the reader could never release follow by scrolling.
  it('subscribes to a container that only appears once there are segments', () => {
    const containerRef: { current: HTMLDivElement | null } = { current: null }
    const onFollowingChange = vi.fn()

    const { rerender } = renderHook(
      ({ count }) =>
        useTranscriptAutoScroll({
          containerRef,
          activeSegmentId: null,
          segmentCount: count,
          isFollowing: true,
          onFollowingChange,
        }),
      { initialProps: { count: 0 } },
    )

    // The transcript arrives: the viewer renders its scroll box, then re-renders.
    const container = createMockContainer()
    containerRef.current = container
    rerender({ count: 3 })

    act(() => {
      Object.defineProperty(container, 'scrollTop', {
        value: 400,
        writable: true,
        configurable: true,
      })
      container.dispatchEvent(new Event('scroll'))
    })

    expect(onFollowingChange).toHaveBeenCalledWith(false)
  })

  it('handles null containerRef gracefully', () => {
    const containerRef = { current: null }
    const onFollowingChange = vi.fn()
    const { result } = renderHook(() =>
      useTranscriptAutoScroll({
        containerRef,
        activeSegmentId: null,
        isFollowing: true,
        onFollowingChange,
      }),
    )

    act(() => {
      result.current.followPlayback()
    })

    expect(onFollowingChange).toHaveBeenCalledWith(true)
  })

  it('unregisters segment ref when called with null element', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const mockElement = createMockElement()

    const { result, rerender } = renderHook(
      ({ activeId }) =>
        useTranscriptAutoScroll({
          containerRef,
          activeSegmentId: activeId,
          isFollowing: true,
          onFollowingChange: vi.fn(),
        }),
      { initialProps: { activeId: null as number | null } },
    )

    act(() => {
      result.current.registerSegmentRef(1, mockElement)
      result.current.registerSegmentRef(1, null)
    })

    rerender({ activeId: 1 })
    expect(container.scrollTo).not.toHaveBeenCalled()
  })

  it('does not report a release during programmatic scroll', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const mockElement = createMockElement()
    const onFollowingChange = vi.fn()

    const { result, rerender } = renderHook(
      ({ activeId }) =>
        useTranscriptAutoScroll({
          containerRef,
          activeSegmentId: activeId,
          isFollowing: true,
          onFollowingChange,
        }),
      { initialProps: { activeId: null as number | null } },
    )

    act(() => {
      result.current.registerSegmentRef(1, mockElement)
    })

    // Trigger auto-scroll by changing active ID
    rerender({ activeId: 1 })

    // Scroll event fires during programmatic scroll — should be ignored
    act(() => {
      container.scrollTop = 500
      container.dispatchEvent(new Event('scroll'))
    })

    expect(onFollowingChange).not.toHaveBeenCalled()
  })

  // The listener reads the live follow state through a ref, so a parent that
  // re-renders with a new inline callback must not leave a stale reporter
  // attached — the release still has to reach the current owner.
  it('reports through the latest callback after a re-render', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const first = vi.fn()
    const second = vi.fn()

    const { rerender } = renderHook(
      ({ report }) =>
        useTranscriptAutoScroll({
          containerRef,
          activeSegmentId: null,
          isFollowing: true,
          onFollowingChange: report,
        }),
      { initialProps: { report: first } },
    )

    rerender({ report: second })

    act(() => {
      container.scrollTop = 200
      container.dispatchEvent(new Event('scroll'))
    })

    expect(first).not.toHaveBeenCalled()
    expect(second).toHaveBeenCalledWith(false)
  })

  it('does not scroll when a transcript arrives after mount and settles on its first turn', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const { result, rerender } = renderHook(
      ({ activeId, count }) =>
        useTranscriptAutoScroll({
          containerRef,
          activeSegmentId: activeId,
          segmentCount: count,
          isFollowing: true,
          onFollowingChange: vi.fn(),
        }),
      { initialProps: { activeId: null as number | null, count: 0 } },
    )
    act(() => result.current.registerSegmentRef(0, createMockElement(0)))
    rerender({ activeId: 0, count: 3 })
    expect(container.scrollTo).not.toHaveBeenCalled()
    // A real change afterwards still follows.
    act(() => result.current.registerSegmentRef(1, createMockElement(400)))
    rerender({ activeId: 1, count: 3 })
    expect(container.scrollTo).toHaveBeenCalledTimes(1)
  })

  it('does not scroll again when only a callback identity changes', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const { result, rerender } = renderHook(
      ({ onChange }) =>
        useTranscriptAutoScroll({
          containerRef,
          activeSegmentId: 1,
          isFollowing: true,
          onFollowingChange: onChange,
        }),
      { initialProps: { onChange: vi.fn() } },
    )
    act(() => result.current.registerSegmentRef(1, createMockElement()))
    rerender({ onChange: vi.fn() })
    expect(container.scrollTo).not.toHaveBeenCalled()
  })

  it('scrolls back to the playing turn when follow is switched back on', () => {
    const container = createMockContainer()
    const containerRef = { current: container }
    const { result, rerender } = renderHook(
      ({ following }) =>
        useTranscriptAutoScroll({
          containerRef,
          activeSegmentId: 1,
          isFollowing: following,
          onFollowingChange: vi.fn(),
        }),
      { initialProps: { following: false } },
    )
    act(() => result.current.registerSegmentRef(1, createMockElement()))
    rerender({ following: true })
    expect(container.scrollTo).toHaveBeenCalledTimes(1)
  })

  describe('window mode (below lg)', () => {
    it('scrolls the window, not the box, to follow playback', () => {
      const container = createMockContainer()
      const containerRef = { current: container }
      const windowScroll = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
      const el = createMockElement()
      el.getBoundingClientRect = () => ({ top: 900, height: 40 }) as DOMRect
      const { result, rerender } = renderHook(
        ({ activeId }) =>
          useTranscriptAutoScroll({
            containerRef,
            activeSegmentId: activeId,
            isFollowing: true,
            onFollowingChange: vi.fn(),
            scrollMode: 'window',
          }),
        { initialProps: { activeId: null as number | null } },
      )
      act(() => result.current.registerSegmentRef(1, el))
      rerender({ activeId: 1 })
      expect(container.scrollTo).not.toHaveBeenCalled()
      expect(windowScroll).toHaveBeenCalledWith({
        top: expect.any(Number),
        behavior: 'smooth',
      })
    })

    function landingTop(el: HTMLElement) {
      const container = createMockContainer()
      const containerRef = { current: container }
      const windowScroll = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
      windowScroll.mockClear()
      const { result, rerender } = renderHook(
        ({ activeId }) =>
          useTranscriptAutoScroll({
            containerRef,
            activeSegmentId: activeId,
            isFollowing: true,
            onFollowingChange: vi.fn(),
            scrollMode: 'window',
          }),
        { initialProps: { activeId: null as number | null } },
      )
      act(() => result.current.registerSegmentRef(1, el))
      rerender({ activeId: 1 })
      return (windowScroll.mock.calls[0]![0] as ScrollToOptions).top!
    }

    it('starts a turn taller than the viewport right under the sticky bars', () => {
      const el = createMockElement()
      el.getBoundingClientRect = () => ({ top: 2000, height: window.innerHeight * 3 }) as DOMRect
      // With no bars set in jsdom the offset is its 16 px of air: 2000 - 16.
      expect(landingTop(el)).toBe(2000 - 16)
    })

    it('still centres a short turn in the space the bars leave free', () => {
      const el = createMockElement()
      el.getBoundingClientRect = () => ({ top: 2000, height: 40 }) as DOMRect
      const free = window.innerHeight - 16
      expect(landingTop(el)).toBe(2000 - (16 + (free - 40) / 2))
    })

    it('lands the turn below the sticky find row', () => {
      const row = document.createElement('div')
      row.setAttribute('data-reader-find', '')
      Object.defineProperty(row, 'offsetHeight', { value: 60 })
      document.body.appendChild(row)
      try {
        const el = createMockElement()
        el.getBoundingClientRect = () => ({ top: 2000, height: window.innerHeight * 3 }) as DOMRect
        expect(landingTop(el)).toBe(2000 - 16 - 60)
      } finally {
        row.remove()
      }
    })

    it('keeps follow on when the page is scrolled while nothing plays', () => {
      const container = createMockContainer()
      const containerRef = { current: container }
      const onFollowingChange = vi.fn()
      renderHook(() =>
        useTranscriptAutoScroll({
          containerRef,
          activeSegmentId: null,
          isFollowing: true,
          onFollowingChange,
          scrollMode: 'window',
          isPlaying: false,
        }),
      )
      act(() => {
        Object.defineProperty(window, 'scrollY', { value: 400, configurable: true })
        window.dispatchEvent(new Event('scroll'))
      })
      expect(onFollowingChange).not.toHaveBeenCalled()
      Object.defineProperty(window, 'scrollY', { value: 0, configurable: true })
    })

    it('releases follow when the page is scrolled by hand', () => {
      const container = createMockContainer()
      const containerRef = { current: container }
      const onFollowingChange = vi.fn()
      renderHook(() =>
        useTranscriptAutoScroll({
          containerRef,
          activeSegmentId: null,
          isFollowing: true,
          onFollowingChange,
          scrollMode: 'window',
        }),
      )
      act(() => {
        Object.defineProperty(window, 'scrollY', { value: 400, configurable: true })
        window.dispatchEvent(new Event('scroll'))
      })
      expect(onFollowingChange).toHaveBeenCalledWith(false)
      Object.defineProperty(window, 'scrollY', { value: 0, configurable: true })
    })

    describe('pressing play', () => {
      let dock: HTMLElement | null = null

      afterEach(() => {
        dock?.remove()
        dock = null
      })

      /** Renders a paused, following window-mode reader whose active turn sits
       *  at `top` in the viewport. jsdom's window is 768 tall. */
      function pausedReader({
        top = 2000,
        activeSegmentId = 1 as number | null,
        isFollowing = true,
        scrollMode = 'window' as 'window' | 'container',
        docked = true,
      } = {}) {
        if (docked && !dock) {
          dock = document.createElement('div')
          dock.dataset.testid = 'reader-mini-dock'
          document.body.append(dock)
        }
        const container = createMockContainer()
        const turn = createMockElement()
        const rect = { top, bottom: top + 40, height: 40 } as DOMRect
        turn.getBoundingClientRect = () => rect
        container.getBoundingClientRect = () => rect
        const containerRef = { current: container }
        const onFollowingChange = vi.fn()
        const hook = renderHook(
          ({ isPlaying }) =>
            useTranscriptAutoScroll({
              containerRef,
              activeSegmentId,
              isFollowing,
              onFollowingChange,
              scrollMode,
              isPlaying,
            }),
          { initialProps: { isPlaying: false } },
        )
        act(() => hook.result.current.registerSegmentRef(1, turn))
        return { ...hook, onFollowingChange }
      }

      it('releases follow when the active turn is below the screen', () => {
        const { rerender, onFollowingChange } = pausedReader({ top: 2000 })
        expect(onFollowingChange).not.toHaveBeenCalled()
        rerender({ isPlaying: true })
        expect(onFollowingChange).toHaveBeenCalledWith(false)
      })

      it('releases follow when the active turn has scrolled above the screen', () => {
        const { rerender, onFollowingChange } = pausedReader({ top: -500 })
        rerender({ isPlaying: true })
        expect(onFollowingChange).toHaveBeenCalledWith(false)
      })

      it('judges the transcript as a whole between turns', () => {
        const away = pausedReader({ top: 2000, activeSegmentId: null })
        away.rerender({ isPlaying: true })
        expect(away.onFollowingChange).toHaveBeenCalledWith(false)

        const here = pausedReader({ top: 200, activeSegmentId: null })
        here.rerender({ isPlaying: true })
        expect(here.onFollowingChange).not.toHaveBeenCalled()
      })

      it('keeps follow when the active turn is on screen', () => {
        const { rerender, onFollowingChange } = pausedReader({ top: 300 })
        rerender({ isPlaying: true })
        expect(onFollowingChange).not.toHaveBeenCalled()
      })

      it('keeps follow when play is pressed on the audio card, with the transcript below the fold', () => {
        const { rerender, onFollowingChange } = pausedReader({ top: 2000, docked: false })
        rerender({ isPlaying: true })
        expect(onFollowingChange).not.toHaveBeenCalled()
      })

      it('does not release again on a pause or on later renders', () => {
        const { rerender, onFollowingChange } = pausedReader({ top: 2000 })
        rerender({ isPlaying: false })
        expect(onFollowingChange).not.toHaveBeenCalled()
        rerender({ isPlaying: true })
        rerender({ isPlaying: true })
        expect(onFollowingChange).toHaveBeenCalledTimes(1)
      })

      it('leaves an already released reader alone', () => {
        const { rerender, onFollowingChange } = pausedReader({ top: 2000, isFollowing: false })
        rerender({ isPlaying: true })
        expect(onFollowingChange).not.toHaveBeenCalled()
      })

      it('changes nothing in container mode', () => {
        const { rerender, onFollowingChange } = pausedReader({ top: 2000, scrollMode: 'container' })
        rerender({ isPlaying: true })
        expect(onFollowingChange).not.toHaveBeenCalled()
      })
    })
  })
})
