import { useCallback, useEffect, useRef, useState, type RefObject } from 'react'
import type { AudioPlayerHandle } from '@/components/media/AudioPlayer'

/** Playback ticks arrive ~4x/second per element; this is the transcript's
 *  highlight budget, not the player's. */
const TIME_UPDATE_INTERVAL_MS = 250

export interface ReaderPlaybackHandlers {
  /** Wired to the audio card. Throttled — see `TIME_UPDATE_INTERVAL_MS`. */
  onTimeUpdate: (time: number) => void
  onPlayStateChange: (playing: boolean) => void
  onDurationChange: (seconds: number) => void
  onSpeedChange: (rate: number) => void
  /** Moves the element. Every seeker goes through here: a transcript turn, a
   *  citation chip, the dock's scrubber. */
  seek: (time: number) => void
  toggle: () => void
  setSpeed: (rate: number) => void
  /** Scrolls the transcript column back into view — the dock's way home. It is
   *  the transcript and not the audio card because below `lg` the card sits
   *  above the words, and "back to transcript" that lands above them is a lie. */
  backToTranscript: () => void
}

export interface ReaderPlayback {
  /** Goes on the audio card, which forwards it to the `<audio>` element. */
  playerRef: RefObject<AudioPlayerHandle | null>
  /** Goes on the card's wrapper; its visibility is what raises the dock. */
  cardRef: RefObject<HTMLDivElement | null>
  /** Goes on the transcript column — where the dock sends the reader back to. */
  transcriptRef: RefObject<HTMLDivElement | null>
  isPlaying: boolean
  currentTime: number
  duration: number
  speed: number
  /** True only once the card has actually scrolled out of view. */
  dockVisible: boolean
  handlers: ReaderPlaybackHandlers
}

/**
 * Everything the Session Reader needs to drive its single `<audio>` element:
 * the handle, the mirrored element state, and the one question the mini dock
 * asks — is the audio card still on screen?
 *
 * It lives outside `SessionReaderPage` because none of it is composition: the
 * page reads the returned values and hands them to the card, the transcript and
 * the dock, and owns no transport logic of its own.
 *
 * @param enabled Whether a player is on the page at all. False keeps the dock
 *   down and skips the observer entirely.
 */
export function useReaderPlayback(enabled: boolean): ReaderPlayback {
  const playerRef = useRef<AudioPlayerHandle>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  const transcriptRef = useRef<HTMLDivElement>(null)
  const lastTick = useRef(0)

  const [currentTime, setCurrentTime] = useState(0)
  const [isPlaying, setIsPlaying] = useState(false)
  const [duration, setDuration] = useState(0)
  const [speed, setSpeed] = useState(1)
  // True until an observer says otherwise, so a browser without
  // IntersectionObserver (and jsdom) simply never shows the dock.
  const [cardInView, setCardInView] = useState(true)

  const onTimeUpdate = useCallback((time: number) => {
    const now = Date.now()
    if (now - lastTick.current < TIME_UPDATE_INTERVAL_MS) return
    lastTick.current = now
    setCurrentTime(time)
  }, [])

  // A seek is not a tick. `AudioPlayer.seek` reports the new position back
  // through `onTimeUpdate`, so without opening the throttle first a transcript
  // click landing less than 250 ms after the previous report would be dropped —
  // leaving the highlighted turn, and the dock's controlled slider, on the old
  // time. Setting the state here as well covers a seek with no element yet.
  const seek = useCallback((time: number) => {
    lastTick.current = 0
    setCurrentTime(time)
    playerRef.current?.seek(time)
  }, [])

  const toggle = useCallback(() => {
    playerRef.current?.toggle()
  }, [])

  const setPlaybackSpeed = useCallback((rate: number) => {
    playerRef.current?.setSpeed(rate)
  }, [])

  const backToTranscript = useCallback(() => {
    // `?.()` and not a call: jsdom leaves scrollIntoView undefined.
    transcriptRef.current?.scrollIntoView?.({ block: 'start', behavior: 'smooth' })
  }, [])

  // The dock exists to replace the card once it has scrolled away, so it is the
  // card's own visibility that drives it — not a scroll offset the layout would
  // invalidate the next time the header grows a line.
  useEffect(() => {
    const node = cardRef.current
    if (!enabled || !node || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[0]
        if (entry) setCardInView(entry.isIntersecting)
      },
      { threshold: 0 },
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [enabled])

  return {
    playerRef,
    cardRef,
    transcriptRef,
    isPlaying,
    currentTime,
    duration,
    speed,
    dockVisible: enabled && !cardInView,
    handlers: {
      onTimeUpdate,
      onPlayStateChange: setIsPlaying,
      onDurationChange: setDuration,
      onSpeedChange: setSpeed,
      seek,
      toggle,
      setSpeed: setPlaybackSpeed,
      backToTranscript,
    },
  }
}
