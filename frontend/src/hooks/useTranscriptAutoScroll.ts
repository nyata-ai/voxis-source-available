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
}

interface UseTranscriptAutoScrollReturn {
  /** Re-engages following and pulls the active segment back into view. */
  followPlayback: () => void
  /** Scrolls to a segment the reader chose (a find-in-transcript match) and
   *  releases playback follow, so the next time update cannot yank the view
   *  back off the match. The inverse of `followPlayback`. */
  jumpToSegment: (id: number) => void
  registerSegmentRef: (id: number, el: HTMLElement | null) => void
}

/** Minimum scroll distance (px) to count as intentional manual scroll. */
const SCROLL_THRESHOLD = 30

/** Time (ms) to keep the auto-scrolling flag active after a programmatic scroll. */
const AUTO_SCROLL_GUARD_MS = 1000

export function useTranscriptAutoScroll({
  containerRef,
  activeSegmentId,
  segmentCount = 1,
  isFollowing,
  onFollowingChange,
}: UseTranscriptAutoScrollOptions): UseTranscriptAutoScrollReturn {
  const hasSegments = segmentCount > 0
  const segmentRefs = useRef<Map<number, HTMLElement>>(new Map())
  const isAutoScrolling = useRef(false)
  const scrollTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastScrollTop = useRef(0)
  const prevActiveId = useRef<number | null | undefined>(undefined)
  // The scroll listener is subscribed once per container. Reading the current
  // follow state and reporter through refs keeps it that way: re-subscribing on
  // every parent render would be a listener churn for no behavioural gain.
  const followingRef = useRef(isFollowing)
  const reportRef = useRef(onFollowingChange)

  useEffect(() => {
    followingRef.current = isFollowing
  }, [isFollowing])

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

  const scrollToSegment = useCallback((id: number) => {
    const container = containerRef.current
    const el = segmentRefs.current.get(id)
    if (!container || !el) return

    // Calculate scroll position to center the element in the container,
    // without using scrollIntoView which also scrolls ancestor containers.
    const targetTop = el.offsetTop - container.offsetTop - (container.clientHeight - el.offsetHeight) / 2
    isAutoScrolling.current = true
    container.scrollTo({ top: targetTop, behavior: 'smooth' })

    if (scrollTimer.current) clearTimeout(scrollTimer.current)
    scrollTimer.current = setTimeout(() => {
      isAutoScrolling.current = false
      // Snapshot scroll position after programmatic scroll settles
      if (container) lastScrollTop.current = container.scrollTop
    }, AUTO_SCROLL_GUARD_MS)
  }, [containerRef])

  const jumpToSegment = useCallback((id: number) => {
    onFollowingChange(false)
    scrollToSegment(id)
  }, [onFollowingChange, scrollToSegment])

  const followPlayback = useCallback(() => {
    onFollowingChange(true)
    if (activeSegmentId !== null) {
      scrollToSegment(activeSegmentId)
    }
  }, [activeSegmentId, onFollowingChange, scrollToSegment])

  // Auto-scroll when active segment changes (skip initial mount to avoid
  // setting the auto-scroll guard before the user has interacted)
  useEffect(() => {
    if (prevActiveId.current === undefined) {
      prevActiveId.current = activeSegmentId
      return
    }
    prevActiveId.current = activeSegmentId
    if (isFollowing && activeSegmentId !== null) {
      scrollToSegment(activeSegmentId)
    }
  }, [activeSegmentId, isFollowing, scrollToSegment])

  // Detect manual scroll to pause auto-follow. Keyed on "are there segments" as
  // well as the ref so a container that only exists once there are segments to
  // show still gets its listener: the ref object is stable, so deps of
  // `[containerRef]` alone subscribe once, against a `current` that was still
  // null. The boolean — not the count — keeps a streaming transcript from
  // re-subscribing on every arriving utterance.
  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const handleScroll = () => {
      // Ignore scroll events caused by programmatic scrollIntoView
      if (isAutoScrolling.current) return

      // Ignore small movements (trackpad jitter, touch bounce)
      const delta = Math.abs(container.scrollTop - lastScrollTop.current)
      if (delta < SCROLL_THRESHOLD) return

      lastScrollTop.current = container.scrollTop
      // Only the transition is worth reporting: a reader scrolling through an
      // already-released transcript should not re-notify on every wheel tick.
      if (followingRef.current) reportRef.current(false)
    }

    container.addEventListener('scroll', handleScroll)
    return () => container.removeEventListener('scroll', handleScroll)
  }, [containerRef, hasSegments])

  // Cleanup scroll timer on unmount
  useEffect(() => {
    return () => {
      if (scrollTimer.current) clearTimeout(scrollTimer.current)
    }
  }, [])

  return { followPlayback, jumpToSegment, registerSegmentRef }
}
