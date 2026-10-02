import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

interface RecordingWaveformProps {
  /** AnalyserNode from the MediaRecorder's audio context. */
  analyserNode: AnalyserNode | null
  /** Whether the recording is paused. */
  isPaused: boolean
}

const BAR_COUNT = 32
const BAR_WIDTH = 3
const BAR_GAP = 2
const MIN_HEIGHT = 2
const MAX_HEIGHT = 48

/**
 * Visualizes audio input as animated bars using AnalyserNode frequency data.
 * Falls back to a CSS-animated placeholder when no AnalyserNode is provided.
 * Flatlines when paused.
 */
export function RecordingWaveform({ analyserNode, isPaused }: RecordingWaveformProps) {
  const { t } = useTranslation('recording')
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const animFrameRef = useRef<number>(0)

  const totalWidth = BAR_COUNT * (BAR_WIDTH + BAR_GAP) - BAR_GAP
  const canvasHeight = MAX_HEIGHT

  useEffect(() => {
    if (!analyserNode || !canvasRef.current) return

    const canvas = canvasRef.current
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const dataArray = new Uint8Array(analyserNode.frequencyBinCount)

    function draw() {
      if (!ctx || !canvasRef.current) return
      animFrameRef.current = requestAnimationFrame(draw)

      analyserNode!.getByteFrequencyData(dataArray)

      ctx.clearRect(0, 0, totalWidth, canvasHeight)

      // Use CSS variable for color (respects theme)
      ctx.fillStyle = isPaused
        ? 'hsl(var(--muted-foreground))'
        : 'hsl(var(--primary))'

      const step = Math.floor(dataArray.length / BAR_COUNT)
      for (let i = 0; i < BAR_COUNT; i++) {
        const value = isPaused ? 0 : dataArray[i * step]
        const normalised = value / 255
        const height = Math.max(MIN_HEIGHT, normalised * MAX_HEIGHT)
        const x = i * (BAR_WIDTH + BAR_GAP)
        const y = (canvasHeight - height) / 2

        ctx.beginPath()
        ctx.roundRect(x, y, BAR_WIDTH, height, 1)
        ctx.fill()
      }
    }

    draw()

    return () => {
      cancelAnimationFrame(animFrameRef.current)
    }
  }, [analyserNode, isPaused, totalWidth, canvasHeight])

  // Fallback: CSS-animated bars when no analyser
  if (!analyserNode) {
    return (
      <div
        className="flex items-center gap-[2px]"
        style={{ height: MAX_HEIGHT }}
        role="img"
        aria-label={t('waveform.ariaLabel')}
      >
        {Array.from({ length: BAR_COUNT }).map((_, i) => (
          <div
            key={i}
            className="w-[3px] rounded-sm bg-muted-foreground/30"
            style={{ height: MIN_HEIGHT }}
          />
        ))}
      </div>
    )
  }

  return (
    <canvas
      ref={canvasRef}
      width={totalWidth}
      height={canvasHeight}
      className="block"
      role="img"
      aria-label={t('waveform.ariaLabel')}
    />
  )
}
