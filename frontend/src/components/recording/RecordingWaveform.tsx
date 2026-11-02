import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { loudnessFromTimeDomain } from './loudness'

interface RecordingWaveformProps {
  /** AnalyserNode from the recorder's (or the mic check's) audio context. */
  analyserNode: AnalyserNode | null
  /** While paused the tape stops rolling and greys out. */
  isPaused: boolean
  /** `test` draws the tape at 40% ink, for the microphone check. */
  tone?: 'live' | 'test'
  /** Called with each new loudness sample (0–1), every SAMPLE_MS. */
  onLevel?: (level: number) => void
}

/** A new loudness bar enters every 100ms. */
const SAMPLE_MS = 100
const BAR_WIDTH = 2
const BAR_GAP = 2
const BAR_STEP = BAR_WIDTH + BAR_GAP
const HEIGHT = 64
const MAX_WIDTH = 640
const MIN_BAR = 2
/** Ring-buffer bound: one bar per 4px of the widest tape. */
const MAX_BARS = MAX_WIDTH / BAR_STEP

function prefersReducedMotion(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function'
    ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
    : false
}

/**
 * The live recording drawn as tape: every 100ms a 2px loudness bar enters on
 * the right and the older ones slide left and fade. The loop samples on a
 * timer rather than every frame, stops while the tab is hidden, and with
 * reduced motion the tape becomes a single level bar that does not travel.
 */
export function RecordingWaveform({ analyserNode, isPaused, tone = 'live', onLevel }: RecordingWaveformProps) {
  const { t } = useTranslation('recording')
  const [reduced] = useState(prefersReducedMotion)
  const [level, setLevel] = useState(0)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const barsRef = useRef<number[]>([])
  const onLevelRef = useRef(onLevel)
  onLevelRef.current = onLevel

  useEffect(() => {
    if (!analyserNode) return
    const samples = new Float32Array(analyserNode.fftSize)
    let timer: number | null = null

    const tick = () => {
      if (!isPaused) {
        analyserNode.getFloatTimeDomainData(samples)
        const next = loudnessFromTimeDomain(samples)
        const bars = barsRef.current
        bars.push(next)
        if (bars.length > MAX_BARS) bars.splice(0, bars.length - MAX_BARS)
        onLevelRef.current?.(next)
        if (reduced) setLevel(next)
      }
      if (!reduced) drawTape(canvasRef.current, barsRef.current)
    }
    const start = () => {
      if (timer === null && document.visibilityState !== 'hidden') {
        timer = window.setInterval(tick, SAMPLE_MS)
      }
    }
    const stop = () => {
      if (timer !== null) window.clearInterval(timer)
      timer = null
    }
    const onVisibility = () => (document.visibilityState === 'hidden' ? stop() : start())

    tick()
    start()
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      stop()
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [analyserNode, isPaused, reduced])

  const colour = cn(
    isPaused ? 'text-muted-foreground' : 'text-foreground',
    tone === 'test' && 'opacity-40',
  )

  if (reduced || !analyserNode) {
    return (
      <div
        role="img"
        aria-label={t('waveform.ariaLabel')}
        className={cn('flex h-16 w-full max-w-[640px] items-center', colour)}
      >
        <div className="h-0.5 w-full rounded-full bg-current opacity-20">
          <div
            className="h-full rounded-full bg-current"
            style={{ width: `${Math.round((analyserNode ? level : 0) * 100)}%` }}
          />
        </div>
      </div>
    )
  }

  return (
    <canvas
      ref={canvasRef}
      role="img"
      aria-label={t('waveform.ariaLabel')}
      className={cn('block h-16 w-full max-w-[640px]', colour)}
    />
  )
}

/**
 * Draws the newest bars right-aligned, older ones fading toward the left.
 * The canvas is sized from its own box each frame, so it fills a phone as
 * well as the 640px desktop width, and stays sharp on high-density screens.
 */
function drawTape(canvas: HTMLCanvasElement | null, bars: number[]): void {
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  const width = Math.min(MAX_WIDTH, Math.floor(canvas.clientWidth)) || MAX_WIDTH
  const ratio = window.devicePixelRatio || 1
  if (canvas.width !== width * ratio || canvas.height !== HEIGHT * ratio) {
    canvas.width = width * ratio
    canvas.height = HEIGHT * ratio
  }
  ctx.setTransform(ratio, 0, 0, ratio, 0, 0)
  ctx.clearRect(0, 0, width, HEIGHT)
  ctx.fillStyle = getComputedStyle(canvas).color

  const visible = Math.min(bars.length, Math.floor(width / BAR_STEP))
  for (let i = 0; i < visible; i++) {
    const value = bars[bars.length - visible + i]
    // Position 0 is the oldest bar on screen; it is the faintest.
    ctx.globalAlpha = 0.15 + 0.85 * ((i + 1) / visible)
    const barHeight = Math.max(MIN_BAR, value * HEIGHT)
    const x = width - (visible - i) * BAR_STEP + BAR_GAP
    ctx.fillRect(x, (HEIGHT - barHeight) / 2, BAR_WIDTH, barHeight)
  }
  ctx.globalAlpha = 1
}
