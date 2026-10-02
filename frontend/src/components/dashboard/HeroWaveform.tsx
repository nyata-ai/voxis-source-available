import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { generateBars } from './waveform-seed'

interface HeroWaveformProps {
  seed: string
  speakers?: number
  durationSeconds?: number
  className?: string
}

const BAR_COUNT = 60
const BAR_WIDTH = 2
const BAR_GAP = 2
const VIEWBOX_WIDTH = BAR_COUNT * (BAR_WIDTH + BAR_GAP)
const VIEWBOX_HEIGHT = 80

export function HeroWaveform({
  seed,
  speakers = 0,
  durationSeconds = 0,
  className,
}: HeroWaveformProps) {
  const { t } = useTranslation('auth')
  const bars = useMemo(
    () => generateBars(seed, BAR_COUNT, { shape: 'hero', saltA: speakers, saltB: durationSeconds }),
    [seed, speakers, durationSeconds]
  )

  const midY = VIEWBOX_HEIGHT / 2

  return (
    <div className={cn('hero-waveform relative h-full w-full text-primary', className)}>
      <svg
        viewBox={`0 0 ${VIEWBOX_WIDTH} ${VIEWBOX_HEIGHT}`}
        preserveAspectRatio="none"
        className="block h-full w-full"
        role="img"
        aria-label={t('visual.audioSignature')}
      >
        {bars.map((amp, i) => {
          const barHeight = amp * (VIEWBOX_HEIGHT - 8)
          const x = i * (BAR_WIDTH + BAR_GAP)
          const y = midY - barHeight / 2

          const t = i / (BAR_COUNT - 1)
          const edgeFade = Math.sin(Math.PI * t)
          const opacity = 0.35 + edgeFade * 0.55

          return (
            <rect
              key={i}
              x={x}
              y={y}
              width={BAR_WIDTH}
              height={barHeight}
              rx={1}
              ry={1}
              fill="currentColor"
              opacity={opacity}
              className="hero-waveform__bar"
            />
          )
        })}
      </svg>

      {/* Playhead overlay — a DOM element so the sweep distance is the rendered card width,
          not the SVG viewBox width (browser-portable across SVG transform semantics). */}
      <span
        aria-hidden="true"
        className="hero-waveform__playhead pointer-events-none absolute inset-y-0 left-0 w-px bg-current opacity-0"
      />
    </div>
  )
}
