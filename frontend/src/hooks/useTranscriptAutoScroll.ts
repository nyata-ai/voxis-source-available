import { useCallback, useEffect, useRef } from 'react'

interface UseTranscriptAutoScrollOptions {
  containerRef: React.RefObject<HTMLDivElement | null>
  activeSegmentId: number | null
  /** Number of segments the caller is rendering. The hook only reads it to know
   *  when the scroll container came into (or went out of) existence: a viewer
   *  that mounts empty and gains utterances later renders its container after
   *  the first effect pass, and a listener subscribed once would never see it.
   *  Defaults to 1 — "assume the container is already there" — for callers whose
   *  container is unconditional. */
  segmentCount?: number
  /** Controlled: true while the list follows playback. The hook never owns this
   *  — the reader's toolbar switch and the viewer's Follow pill drive the same
   *  value, so a single owner above both is the only way they agree. */
  isFollowing: boolean
  /** Reports a follow-state change the hook detected or performed: `false` when
   *  the reader scrolls away or jumps to a match, `true` from `followPlayback`. */
  onFollowingChange: (following: boolean) => void
  /** Who scrolls. `container` (the default, desktop): the transcript's own
   *  box. `window`: below `lg` the box has no height cap, so the page itself
   *  scrolls and following moves the window. */
  scrollMode?: ScrollMode
  /** Whether the audio is playing. In window mode the page is the only scroll,
   *  so scrolling it while nothing plays is ordinary reading, not leaving
   *  playback: it must not switch follow off before anyone pressed play. Pressing
   *  play in the mini dock with the turn off-screen is what releases it.
   *  Defaults to true, the box's behaviour: any manual scroll releases. */
  isPlaying?: boolean
}

export type ScrollMode = 'container' | 'window'

interface UseTranscriptAutoScrollReturn {
  /** Re-engages following and pulls the active segment back into view. */
  followPlayback: () => void
  /** Scrolls to a segment the reader chose (a find-in-transcript match) and
   *  releases playback follow, so the next time update cannot yank the view
   *  back off the match. The inverse of `followPlayback`. */
  jumpToSegment: (id: number) => void
  /** Brings a segment on screen without touching follow — a citation landing.
   *  In container mode it also scrolls the page to the box, because the reader
   *  clicked from the briefings below it. */
  revealSegment: (id: number) => void
  registerSegmentRef: (id: number, el: HTMLElement | null) => void
}

/** Minimum scroll distance (px) to count as intentional manual scroll. */
const SCROLL_THRESHOLD = 30

/** Time (ms) to keep the auto-scrolling flag active after a programmatic scroll. */
const AUTO_SCROLL_GUARD_MS = 1000

/** Smooth unless the reader asked for reduced motion. JS `behavior` is not
 *  covered by the global CSS `scroll-behavior` override, so it is asked here. */
export function scrollBehavior(): ScrollBehavior {
  const reduce =
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  return reduce ? 'auto' : 'smooth'
}

/** How much of the viewport's top the sticky bars cover at `el`: the app
 *  header plus the reader's bar, read from the same custom properties (rem)
 *  the layout uses. 0 where they are not set. */
function stickyOffset(el: HTMLElement): number {
  const style = getComputedStyle(el)
  const rem = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16
  const read = (name: string) => parseFloat(style.getPropertyValue(name)) || 0
  return (read('--app-header-h') + read('--reader-nav-h')) * rem + 16
}

/** Below `lg` the find row sticks under the bars; it covers the page too.
 *  Read from the DOM because it wraps to more rows on some widths. */
function findRowHeight(): number {
  return document.querySelector<HTMLElement>('[data-reader-find]')?.offsetHeight ?? 0
}

/**
 * Where a turn's top should land when the window follows: centred in what the
 * sticky bars leave free, except that a turn taller than that space starts
 * right under them, so its first words are never above the screen.
 */
function windowLandingY(el: HTMLElement, height: number): number {
  const covered = stickyOffset(el) + findRowHeight()
  return covered + Math.max(0, (window.innerHeight - covered - height) / 2)
}

/** Whether any of `el` shows in what the sticky bars leave free. */
function isOnScreen(el: HTMLElement): boolean {
  const rect = el.getBoundingClientRect()
  return rect.bottom > stickyOffset(el) + findRowHeight() && rect.top < window.innerHeight
}

/** The mini dock shows only once the audio card has left the screen, so it says
 *  the reader has moved away from the head of the page. */
function isMiniDockShowing(): boolean {
  return document.querySelector('[data-testid="reader-mini-dock"]') !== null
}

/** The scroll position the manual-scroll check compares against. */
function readScrollTop(mode: ScrollMode, container: HTMLElement | null): number {
  if (mode === 'window') return window.scrollY
  return container?.scrollTop ?? 0
}

export function useTranscriptAutoScroll({
  containerRef,
  activeSegmentId,
  segmentCount = 1,
  isFollowing,
  onFollowingChange,
  scrollMode = 'container',
  isPlaying = true,
}: UseTranscriptAutoScrollOptions): UseTranscriptAutoScrollReturn {
  const hasSegments = segmentCount > 0
  const segmentRefs = useRef<Map<number, HTMLElement>>(new Map())
  const isAutoScrolling = useRef(false)
  const scrollTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastScrollTop = useRef(0)
  const prevActiveId = useRef<number | null | undefined>(undefined)
  const prevFollowing = useRef(isFollowing)
  // The scroll listener is subscribed once per container. Reading the current
  // follow state and reporter through refs keeps it that way: re-subscribing on
  // every parent render would be a listener churn for no behavioural gain.
  const followingRef = useRef(isFollowing)
  const reportRef = useRef(onFollowingChange)
  const playingRef = useRef(isPlaying)
  const prevPlaying = useRef(isPlaying)

  useEffect(() => {
    followingRef.current = isFollowing
  }, [isFollowing])

  useEffect(() => {
    playingRef.current = isPlaying
  }, [isPlaying])

  useEffect(() => {
    reportRef.current = onFollowingChange
  }, [onFollowingChange])

  const registerSegmentRef = useCallback((id: number, el: HTMLElement | null) => {
    if (el) {
      segmentRefs.current.set(id, el)
    } else {
      segmentRefs.current.delete(id)
    }
  }, [])

  /** Holds the manual-scroll check off while a programmatic scroll settles,
   *  then snapshots where it landed. */
  const guardAutoScroll = useCallback(() => {
    isAutoScrolling.current = true
    if (scrollTimer.current) clearTimeout(scrollTimer.current)
    scrollTimer.current = setTimeout(() => {
      isAutoScrolling.current = false
      lastScrollTop.current = readScrollTop(scrollMode, containerRef.current)
    }, AUTO_SCROLL_GUARD_MS)
  }, [containerRef, scrollMode])

  const scrollToSegment = useCallback((id: number) => {
    const container = containerRef.current
    const el = segmentRefs.current.get(id)
    if (!container || !el) return

    if (scrollMode === 'window') {
      // The page, not the box, is what moves.
      const rect = el.getBoundingClientRect()
      const top = window.scrollY + rect.top - windowLandingY(el, rect.height)
      guardAutoScroll()
      window.scrollTo({ top: Math.max(0, top), behavior: scrollBehavior() })
      return
    }

    // Calculate scroll position to center the element in the container,
    // without using scrollIntoView which also scrolls ancestor containers.
    const targetTop = el.offsetTop - container.offsetTop - (container.clientHeight - el.offsetHeight) / 2
    guardAutoScroll()
    container.scrollTo({ top: targetTop, behavior: scrollBehavior() })
  }, [containerRef, scrollMode, guardAutoScroll])

  const jumpToSegment = useCallback((id: number) => {
    onFollowingChange(false)
    scrollToSegment(id)
  }, [onFollowingChange, scrollToSegment])

  const revealSegment = useCallback((id: number) => {
    scrollToSegment(id)
    const container = containerRef.current
    if (scrollMode !== 'container' || !container) return
    // Bring the box itself on screen when it is not (the reader clicked from
    // the briefings below it). Not scrollIntoView: that also scrolls clipped
    // ancestors, which shifts the whole app shell.
    const rect = container.getBoundingClientRect()
    if (rect.top >= 0 && rect.top < window.innerHeight / 2) return
    const top = window.scrollY + rect.top - stickyOffset(container)
    window.scrollTo({ top: Math.max(0, top), behavior: scrollBehavior() })
  }, [containerRef, scrollMode, scrollToSegment])

  const followPlayback = useCallback(() => {
    onFollowingChange(true)
    if (activeSegmentId !== null) {
      scrollToSegment(activeSegmentId)
    }
  }, [activeSegmentId, onFollowingChange, scrollToSegment])

  // Auto-scroll when active segment changes. The first value seen once there
  // are segments is where the reader starts, not a change: a transcript that
  // arrives after the viewer mounted moves the active turn from "none" to 0,
  // and following that in window mode would scroll a phone's page past the
  // header before anyone pressed play.
  //
  // Only a real change scrolls: the active turn moving, or follow switching
  // back on. A re-run for any other reason (a callback's identity, the scroll
  // mode flipping at a breakpoint) must not move the page under the reader.
  useEffect(() => {
    const wasFollowing = prevFollowing.current
    prevFollowing.current = isFollowing
    if (prevActiveId.current === undefined) {
      if (hasSegments) prevActiveId.current = activeSegmentId
      return
    }
    const moved = prevActiveId.current !== activeSegmentId
    prevActiveId.current = activeSegmentId
    if (!isFollowing || activeSegmentId === null) return
    if (moved || !wasFollowing) scrollToSegment(activeSegmentId)
  }, [activeSegmentId, isFollowing, scrollToSegment, hasSegments])

  // Window mode only. Scrolling the page while paused keeps follow on (see the
  // manual-scroll check below), so a reader can press play in the mini dock far
  // from the transcript. Their position wins: following on from there would move
  // the whole page at the next turn. Between turns there is no active turn, so
  // the transcript as a whole stands in for it. Playing from the audio card is
  // different: on most phones the transcript starts below the fold, and follow
  // is what carries the reader down to it. A programmatic scroll still in flight
  // is on its way to the turn and does not count as being elsewhere.
  useEffect(() => {
    const started = isPlaying && !prevPlaying.current
    prevPlaying.current = isPlaying
    if (!started || scrollMode !== 'window' || !isFollowing) return
    if (isAutoScrolling.current || !isMiniDockShowing()) return
    const turn = activeSegmentId === null ? undefined : segmentRefs.current.get(activeSegmentId)
    const target = turn ?? containerRef.current
    if (target && !isOnScreen(target)) onFollowingChange(false)
  }, [isPlaying, isFollowing, activeSegmentId, scrollMode, containerRef, onFollowingChange])

  // Detect manual scroll to pause auto-follow. Keyed on "are there segments" as
  // well as the ref so a container that only exists once there are segments to
  // show still gets its listener: the ref object is stable, so deps of
  // `[containerRef]` alone subscribe once, against a `current` that was still
  // null. The boolean — not the count — keeps a streaming transcript from
  // re-subscribing on every arriving utterance. In window mode the page's own
  // scroll is the one that counts.
  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const target: HTMLElement | Window = scrollMode === 'window' ? window : container
    lastScrollTop.current = readScrollTop(scrollMode, container)

    const handleScroll = () => {
      // Ignore scroll events caused by programmatic scrolling
      if (isAutoScrolling.current) return

      // Ignore small movements (trackpad jitter, touch bounce)
      const top = readScrollTop(scrollMode, container)
      if (Math.abs(top - lastScrollTop.current) < SCROLL_THRESHOLD) return

      lastScrollTop.current = top
      if (scrollMode === 'window' && !playingRef.current) return
      // Only the transition is worth reporting: a reader scrolling through an
      // already-released transcript should not re-notify on every wheel tick.
      if (followingRef.current) reportRef.current(false)
    }

    target.addEventListener('scroll', handleScroll, { passive: true })
    return () => target.removeEventListener('scroll', handleScroll)
  }, [containerRef, hasSegments, scrollMode])

  // Cleanup scroll timer on unmount
  useEffect(() => {
    return () => {
      if (scrollTimer.current) clearTimeout(scrollTimer.current)
    }
  }, [])

  return { followPlayback, jumpToSegment, revealSegment, registerSegmentRef }
}
