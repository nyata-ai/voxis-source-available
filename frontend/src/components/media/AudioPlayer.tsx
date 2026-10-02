import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Pause, Play, RotateCcw, RotateCw, Volume2, VolumeX } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Slider } from '@/components/ui/slider'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { usePreferences } from '@/hooks/useSettings'
import { formatDuration } from '@/lib/utils'
import { speedRungs } from './playback-speeds'

export interface AudioPlayerHandle {
  seek: (time: number) => void
  /** Play or pause, whichever the element is not doing. */
  toggle: () => void
  /** Sets the playback rate. Ignores anything that is not a positive number —
   *  a bad rate would silence the element rather than fail loudly. */
  setSpeed: (rate: number) => void
}

/** Step size for the skip buttons. */
const SKIP_SECONDS = 5

/**
 * The 3 px scrubber shape both of the reader's transports use: coral fill, a
 * small handle kept visible because it is the keyboard target. The track and
 * handle colours belong to the surface — see `INK_SLIDER` here and the mini
 * dock's paper variant — so this holds only what the two share.
 */
export const THIN_SLIDER =
  'py-2 [&>span:first-child]:h-[3px] [&>span:first-child>span]:bg-coral ' +
  '[&_[role=slider]]:h-3 [&_[role=slider]]:w-3'

/** `THIN_SLIDER` on ink: translucent track, white handle. */
const INK_SLIDER =
  `${THIN_SLIDER} [&>span:first-child]:bg-white/30 ` +
  '[&_[role=slider]]:border-white [&_[role=slider]]:bg-white'

/** The ink card's own focus ring. `ring-ring` is near-black in the light
 *  theme — the exact colour of the card behind it, so the ring disappears. */
const INK_FOCUS = 'focus-visible:ring-white'

/** What the card needs from the element's state. Kept as one object so the
 *  layout stays short and does not own playback. */
interface PlayerViewProps {
  src: string | null
  isLoading: boolean
  isPlaying: boolean
  currentTime: number
  duration: number
  isMuted: boolean
  speed: number
  speedOptions: number[]
  onToggle: () => void
  onSkip: (delta: number) => void
  onScrub: (value: number[]) => void
  onSpeedChange: (rate: number) => void
  onToggleMute: () => void
}

export interface SpeedSelectProps {
  speed: number
  /** The rungs to offer. Defaults to the rungs for `speed`, which is what every
   *  caller wants; passing them in only saves recomputing them. */
  speedOptions?: number[]
  onSpeedChange: (rate: number) => void
  className?: string
  /** Overrides the trigger's text — the ink card says "1x speed", the dock "1x". */
  label?: string
}

/**
 * The playback-rate control. Exported because the reader has two transports —
 * the audio card and the mini dock — and they must never offer different rungs
 * or name the speed differently.
 */
export function SpeedSelect({
  speed,
  speedOptions,
  onSpeedChange,
  className,
  label,
}: SpeedSelectProps) {
  const { t } = useTranslation('media')
  const options = speedOptions ?? speedRungs(speed)
  return (
    <Select value={String(speed)} onValueChange={(value) => onSpeedChange(Number(value))}>
      <SelectTrigger className={className} aria-label={t('player.speed')}>
        <SelectValue>{label ?? t('player.speedValue', { speed })}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option} value={String(option)}>
            {t('player.speedValue', { speed: option })}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

/** The reader's ink audio card — the page's primary player. */
function CardLayout(props: PlayerViewProps) {
  const { t } = useTranslation('media')
  const { src, isLoading, isPlaying, currentTime, duration, isMuted } = props
  const disabled = !src || isLoading
  const skipClass = `h-11 w-11 rounded-full text-white/80 hover:bg-white/10 hover:text-white sm:h-9 sm:w-9 ${INK_FOCUS}`

  return (
    <div className="flex flex-col gap-4 rounded-lg bg-ink p-5 text-white dark:bg-card">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-2 sm:gap-3">
          <Button
            variant="ghost"
            size="icon"
            className={skipClass}
            onClick={() => props.onSkip(-SKIP_SECONDS)}
            disabled={disabled}
            aria-label={t('player.skipBack', { seconds: SKIP_SECONDS })}
          >
            <RotateCcw className="h-5 w-5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className={`h-12 w-12 rounded-full border-[1.5px] border-white text-white hover:bg-white/10 hover:text-white ${INK_FOCUS}`}
            onClick={props.onToggle}
            disabled={disabled}
            aria-label={isPlaying ? t('player.pause') : t('player.play')}
          >
            {isPlaying ? <Pause className="h-5 w-5" /> : <Play className="h-5 w-5" />}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className={skipClass}
            onClick={() => props.onSkip(SKIP_SECONDS)}
            disabled={disabled}
            aria-label={t('player.skipForward', { seconds: SKIP_SECONDS })}
          >
            <RotateCw className="h-5 w-5" />
          </Button>
        </div>
        {/* No live region: this ticks four times a second, and a screen reader
            announcing every tick would bury everything else on the page. */}
        <span className="shrink-0 tabular-nums text-xs text-white/70">
          {formatDuration(currentTime)} / {formatDuration(duration)}
        </span>
      </div>

      <Slider
        value={[currentTime]}
        max={duration || 100}
        step={0.1}
        onValueChange={props.onScrub}
        disabled={disabled}
        aria-label={t('player.position')}
        className={INK_SLIDER}
      />

      <div className="flex items-center justify-between gap-3">
        <SpeedSelect
          speed={props.speed}
          speedOptions={props.speedOptions}
          onSpeedChange={props.onSpeedChange}
          label={t('player.speedPill', { speed: props.speed })}
          className="h-11 w-auto gap-1.5 rounded-full border-white/30 bg-transparent px-3.5 text-xs font-semibold text-white focus:ring-white sm:h-9"
        />
        <Button
          variant="ghost"
          size="icon"
          className={`h-11 w-11 text-white/80 hover:bg-white/10 hover:text-white sm:h-9 sm:w-9 ${INK_FOCUS}`}
          onClick={props.onToggleMute}
          disabled={!src}
          aria-label={isMuted ? t('player.unmute') : t('player.mute')}
        >
          {isMuted ? <VolumeX className="h-5 w-5" /> : <Volume2 className="h-5 w-5" />}
        </Button>
      </div>
    </div>
  )
}

interface AudioPlayerProps {
  src: string | null
  isLoading?: boolean
  onError?: () => void
  /** Fired once the element has metadata for `src` — i.e. the URL is good.
   *  Callers that count stream failures use it to retire the count. */
  onLoaded?: () => void
  onTimeUpdate?: (time: number) => void
  /** Mirrors play/pause outward so a second, presentational control (the
   *  reader's mini dock) can show the right glyph without a second element. */
  onPlayStateChange?: (isPlaying: boolean) => void
  /** Reports the element's duration once metadata lands, and 0 while unknown. */
  onDurationChange?: (seconds: number) => void
  /** Reports the playback rate, so a second control cannot show a speed the
   *  audio is not playing at. */
  onSpeedChange?: (rate: number) => void
}

export const AudioPlayer = forwardRef<AudioPlayerHandle, AudioPlayerProps>(function AudioPlayer(
  {
    src,
    isLoading = false,
    onError,
    onLoaded,
    onTimeUpdate,
    onPlayStateChange,
    onDurationChange,
    onSpeedChange,
  },
  ref,
) {
  const audioRef = useRef<HTMLAudioElement>(null)
  const [isPlaying, setIsPlaying] = useState(false)
  const [currentTime, setCurrentTime] = useState(0)
  const [duration, setDuration] = useState(0)
  const [isMuted, setIsMuted] = useState(false)
  const [speed, setSpeed] = useState(1)

  const { data: preferences } = usePreferences()
  const preferredSpeed = preferences?.playback_speed
  // The preference seeds the initial rate once and is never written back — a
  // per-session speed change is not a settings change.
  const seededSpeed = useRef(false)
  useEffect(() => {
    if (seededSpeed.current || preferredSpeed === undefined || preferredSpeed <= 0) return
    seededSpeed.current = true
    setSpeed(preferredSpeed)
  }, [preferredSpeed])

  const seekTo = useCallback((time: number) => {
    const audio = audioRef.current
    if (!audio) return
    audio.currentTime = time
    setCurrentTime(time)
    onTimeUpdate?.(time)
  }, [onTimeUpdate])

  // The element's own `play`/`pause` events own `isPlaying` — an optimistic
  // flip here could never be corrected, and `play()` rejects more often than it
  // looks: AbortError when a second toggle interrupts it, NotSupportedError on
  // an expired signed URL. A rejected play() fires no `pause` of its own, so
  // that one case settles the state explicitly.
  const togglePlay = useCallback(() => {
    const audio = audioRef.current
    if (!audio) return
    if (isPlaying) {
      audio.pause()
      return
    }
    void audio.play().catch(() => setIsPlaying(false))
  }, [isPlaying])

  const changeSpeed = useCallback((rate: number) => {
    if (!Number.isFinite(rate) || rate <= 0) return
    setSpeed(rate)
  }, [])

  useImperativeHandle(
    ref,
    () => ({ seek: seekTo, toggle: togglePlay, setSpeed: changeSpeed }),
    [seekTo, togglePlay, changeSpeed],
  )

  // A new source is a new recording: everything the old one reported is stale.
  useEffect(() => {
    setIsPlaying(false)
    setCurrentTime(0)
    setDuration(0)
  }, [src])

  useEffect(() => {
    const audio = audioRef.current
    if (audio) audio.playbackRate = speed
  }, [speed, src])

  // Reported rather than pushed from each handler: one place to change, and a
  // consumer can never miss a transition (an `ended` event included).
  useEffect(() => { onPlayStateChange?.(isPlaying) }, [isPlaying, onPlayStateChange])
  useEffect(() => { onDurationChange?.(duration) }, [duration, onDurationChange])
  useEffect(() => { onSpeedChange?.(speed) }, [speed, onSpeedChange])

  const toggleMute = useCallback(() => {
    const audio = audioRef.current
    if (!audio) return
    audio.muted = !isMuted
    setIsMuted(!isMuted)
  }, [isMuted])

  const skip = useCallback((delta: number) => {
    // An unknown duration (metadata not loaded) has no upper bound to clamp to.
    const upper = duration > 0 ? duration : Number.POSITIVE_INFINITY
    seekTo(Math.min(Math.max(currentTime + delta, 0), upper))
  }, [currentTime, duration, seekTo])

  const handleTimeUpdate = useCallback(() => {
    const audio = audioRef.current
    if (audio) {
      setCurrentTime(audio.currentTime)
      onTimeUpdate?.(audio.currentTime)
    }
  }, [onTimeUpdate])

  const handleLoadedMetadata = useCallback(() => {
    const audio = audioRef.current
    // A live stream reports Infinity and a truncated header NaN; either would
    // reach the card and the mini dock as "NaN:NaN". 0 means "unknown", which
    // both layouts already handle.
    if (audio) {
      const reported = audio.duration
      setDuration(Number.isFinite(reported) && reported > 0 ? reported : 0)
    }
    onLoaded?.()
  }, [onLoaded])

  const handleEnded = useCallback(() => {
    setIsPlaying(false)
    setCurrentTime(0)
  }, [])

  const handleScrub = useCallback((value: number[]) => {
    if (value[0] !== undefined) seekTo(value[0])
  }, [seekTo])

  const speedOptions = useMemo(() => speedRungs(speed), [speed])

  if (!src && !isLoading) return null

  const view: PlayerViewProps = {
    src,
    isLoading,
    isPlaying,
    currentTime,
    duration,
    isMuted,
    speed,
    speedOptions,
    onToggle: togglePlay,
    onSkip: skip,
    onScrub: handleScrub,
    onSpeedChange: changeSpeed,
    onToggleMute: toggleMute,
  }

  return (
    <>
      {src && (
        <audio
          ref={audioRef}
          src={src}
          onTimeUpdate={handleTimeUpdate}
          onLoadedMetadata={handleLoadedMetadata}
          onEnded={handleEnded}
          onPlay={() => setIsPlaying(true)}
          onPause={() => setIsPlaying(false)}
          onError={onError}
          preload="metadata"
        />
      )}
      <CardLayout {...view} />
    </>
  )
})
