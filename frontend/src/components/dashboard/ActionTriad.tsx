import { useId, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowUpRight, Mic, Upload as UploadIcon, type LucideIcon } from 'lucide-react'
import { useCaptureStore } from '@/stores/capture'
import { INTAKE_DISC_CLASS, type IntakeChannel } from '@/lib/intake-tone'
import { cn } from '@/lib/utils'
import type { CaptureTab } from '@/config/routes'

interface Channel {
  tab: CaptureTab
  tone: IntakeChannel
  icon: LucideIcon
  glyph: ReactNode
  catalogKey: string
  titleKey: string
  descriptionKey: string
}

const CHANNELS: readonly Channel[] = [
  {
    tab: 'record',
    tone: 'record',
    icon: Mic,
    glyph: <RecordGlyph />,
    catalogKey: 'dashboard.recordCatalog',
    titleKey: 'dashboard.record',
    descriptionKey: 'dashboard.recordBody',
  },
  {
    tab: 'upload',
    tone: 'upload',
    icon: UploadIcon,
    glyph: <UploadGlyph />,
    catalogKey: 'dashboard.uploadCatalog',
    titleKey: 'dashboard.upload',
    descriptionKey: 'dashboard.uploadBody',
  },
]

interface ActionTriadProps {
  /**
   * `full`: the two large illustrated tiles, for a first visit (rows on
   * phones). `compact`: one slim ~72px strip, for returning users whose Desk
   * leads with their own work instead.
   */
  variant?: 'full' | 'compact'
}

// The dashboard's two intake channels. Each cell opens the shared capture
// modal on its tab — the same dialog the header quick actions and the
// `?capture=` deep link open, so there is one way to start a transcription.
// Nothing here animates on load; the glyphs only move on hover.
export function ActionTriad({ variant = 'full' }: ActionTriadProps) {
  const { t } = useTranslation('oss')
  const openCapture = useCaptureStore((s) => s.openCapture)
  const compact = variant === 'compact'

  return (
    <section aria-labelledby="action-triad-heading">
      <div className={cn('mb-3 flex items-baseline justify-between gap-3', !compact && 'sr-only')}>
        <h2 id="action-triad-heading" className="kicker">
          {t('dashboard.title')}
        </h2>
        <p className="hidden text-body text-muted-foreground sm:block">{t('dashboard.description')}</p>
      </div>

      {/* The compact strip waits for `lg`, like the first-visit tiles at `md`,
          so its text keeps room beside the sidebar. */}
      <div
        className={cn(
          'grid grid-cols-1 divide-y divide-border/60 overflow-hidden rounded-lg border border-border bg-card',
          compact ? 'lg:grid-cols-2 lg:divide-x lg:divide-y-0' : 'md:grid-cols-2 md:divide-x md:divide-y-0',
        )}
      >
        {CHANNELS.map((channel) => (
          <ActionCell
            key={channel.tab}
            channel={channel}
            compact={compact}
            catalog={t(channel.catalogKey)}
            title={t(channel.titleKey)}
            description={t(channel.descriptionKey)}
            onClick={() => openCapture(channel.tab)}
          />
        ))}
      </div>
    </section>
  )
}

interface ActionCellProps {
  channel: Channel
  compact: boolean
  catalog: string
  title: string
  description: string
  onClick: () => void
}

function ActionCell({ channel, compact, catalog, title, description, onClick }: ActionCellProps) {
  // Stable ids per render so screen readers see both title and description as the
  // cell's accessible name + description, instead of an aria-label that would
  // override and hide the visible description text.
  const baseId = useId()
  const titleId = `${baseId}-title`
  const descId = `${baseId}-desc`
  const Icon = channel.icon
  const disc = INTAKE_DISC_CLASS[channel.tone]

  const focusRing =
    'focus:outline-none focus-visible:bg-accent/40 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary'

  if (compact) {
    return (
      <button
        type="button"
        onClick={onClick}
        className={cn(
          'group flex min-h-18 items-center gap-3 px-4 py-2.5 text-left transition-colors duration-200 ease-out hover:bg-accent/40',
          focusRing
        )}
        aria-labelledby={titleId}
        aria-describedby={descId}
      >
        <span
          className={cn('flex h-10 w-10 shrink-0 items-center justify-center rounded-full', disc)}
          aria-hidden="true"
        >
          <Icon className="h-4 w-4" />
        </span>
        <span className="min-w-0 flex-1">
          <span id={titleId} className="line-clamp-2 text-title text-foreground">
            {title}
          </span>
          <span id={descId} title={description} className="block truncate text-body text-muted-foreground">
            {description}
          </span>
        </span>
        <ArrowUpRight
          className="h-4 w-4 shrink-0 text-muted-foreground transition-colors duration-200 group-hover:text-foreground"
          aria-hidden="true"
        />
      </button>
    )
  }

  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        // Phones: a row (64px disc left, text right). md+: the illustrated tile.
        'group relative flex h-full items-center gap-4 p-4 text-left md:flex-col md:items-stretch md:gap-5',
        'transition-colors duration-200 ease-out hover:bg-accent/40',
        focusRing
      )}
      aria-labelledby={titleId}
      aria-describedby={descId}
    >
      <span className="kicker hidden transition-colors group-hover:text-foreground md:block">{catalog}</span>

      {/* Illustration zone — the channel's intake colour behind the glyph */}
      <div
        className={cn(
          'relative flex h-16 w-16 shrink-0 items-center justify-center rounded-lg md:h-28 md:w-full md:px-3',
          disc
        )}
        aria-hidden="true"
      >
        <Icon className="h-6 w-6 md:hidden" />
        <span className="hidden h-full w-full items-center justify-center md:flex">{channel.glyph}</span>
      </div>

      {/* Title + supporting line + CTA arrow */}
      <div className="flex min-w-0 flex-1 items-center justify-between gap-3 md:mt-auto md:items-end">
        <div className="min-w-0 space-y-1.5">
          <h3
            id={titleId}
            className="text-section text-foreground transition-colors duration-200 group-hover:text-primary"
          >
            {title}
          </h3>
          <p id={descId} className="text-body text-muted-foreground">
            {description}
          </p>
        </div>
        <ArrowUpRight
          className="h-5 w-5 shrink-0 text-muted-foreground transition-colors duration-200 group-hover:text-primary"
          aria-hidden="true"
        />
      </div>
    </button>
  )
}

/* ───────────────────────────── Custom SVG glyphs ─────────────────────────────
   Each glyph is mono-line, drawn in currentColor, and animates only on hover
   via classes that hook into globals.css keyframes. The glyphs share a
   common terminus: a miniature waveform on the right edge — visually unifying
   the intake channels around the project's signature audio motif.
   ─────────────────────────────────────────────────────────────────────────── */

const TERMINUS_BARS: Array<[number, number]> = [
  [0, 16], [6, 32], [12, 20], [18, 44], [24, 24],
  [30, 36], [36, 18], [42, 28], [48, 12], [54, 20],
]

// Right-edge mini waveform shared by both glyphs. Named "terminus" because
// it's the common endpoint that visually unifies the channels.
function TerminusWaveform({ x, y, opacity = 0.85 }: { x: number; y: number; opacity?: number }) {
  return (
    <g transform={`translate(${x}, ${y})`} opacity={opacity}>
      {TERMINUS_BARS.map(([dx, h]) => (
        <rect
          key={dx}
          x={dx}
          y={-h / 2}
          width={2}
          height={h}
          fill="currentColor"
          rx={0.5}
        />
      ))}
    </g>
  )
}

// Stagger interval ≈ one beat of the 1.6s arc loop — gives a "ripple outward"
// feel as each arc fires slightly later than the one inside it.
const ARC_STAGGER_MS = 180

function RecordGlyph() {
  return (
    <svg
      viewBox="0 0 240 110"
      preserveAspectRatio="xMidYMid meet"
      className="block h-full w-auto max-w-full"
      aria-hidden="true"
    >
      {/* Microphone capsule */}
      <rect
        x={42}
        y={18}
        width={22}
        height={44}
        rx={11}
        ry={11}
        fill="none"
        stroke="currentColor"
        strokeWidth={1.5}
      />
      {/* Grille bands */}
      <line x1={47} y1={30} x2={59} y2={30} stroke="currentColor" strokeWidth={0.75} opacity={0.55} />
      <line x1={47} y1={38} x2={59} y2={38} stroke="currentColor" strokeWidth={0.75} opacity={0.55} />
      <line x1={47} y1={46} x2={59} y2={46} stroke="currentColor" strokeWidth={0.75} opacity={0.55} />
      {/* Yoke */}
      <path
        d="M 33 48 V 56 A 20 20 0 0 0 73 56 V 48"
        fill="none"
        stroke="currentColor"
        strokeWidth={1.5}
      />
      {/* Stand */}
      <line x1={53} y1={76} x2={53} y2={94} stroke="currentColor" strokeWidth={1.5} />
      <line x1={38} y1={94} x2={68} y2={94} stroke="currentColor" strokeWidth={1.5} />

      {/* Broadcast arcs — open rightward, pulse on hover with a stagger */}
      <path
        d="M 92 32 Q 110 55 92 78"
        fill="none"
        stroke="currentColor"
        strokeWidth={1.25}
        opacity={0.85}
        className="action-glyph__arc"
      />
      <path
        d="M 118 22 Q 144 55 118 88"
        fill="none"
        stroke="currentColor"
        strokeWidth={1.25}
        opacity={0.55}
        className="action-glyph__arc"
        style={{ animationDelay: `${ARC_STAGGER_MS}ms` }}
      />
      <path
        d="M 144 12 Q 178 55 144 98"
        fill="none"
        stroke="currentColor"
        strokeWidth={1.25}
        opacity={0.3}
        className="action-glyph__arc"
        style={{ animationDelay: `${ARC_STAGGER_MS * 2}ms` }}
      />

      {/* Unifying terminus waveform */}
      <TerminusWaveform x={184} y={55} />

      {/* "REC" tick — tiny editorial mark; pulses on hover */}
      <circle cx={53} cy={11} r={2.4} fill="currentColor" className="action-glyph__rec-dot" />
    </svg>
  )
}

// 150ms stagger keeps three arrows bobbing in counterpoint without overlapping.
const INTAKE_STAGGER_MS = 150

function UploadGlyph() {
  return (
    <svg
      viewBox="0 0 240 110"
      preserveAspectRatio="xMidYMid meet"
      className="block h-full w-auto max-w-full"
      aria-hidden="true"
    >
      {/* Three downward intake arrows — bob on hover */}
      <g className="action-glyph__intake-arrows">
        <g>
          <line x1={88} y1={4} x2={88} y2={20} stroke="currentColor" strokeWidth={1.25} opacity={0.5} />
          <polyline
            points="84,15 88,20 92,15"
            fill="none"
            stroke="currentColor"
            strokeWidth={1.25}
            opacity={0.5}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </g>
        <g style={{ animationDelay: `${INTAKE_STAGGER_MS}ms` }}>
          <line x1={110} y1={2} x2={110} y2={22} stroke="currentColor" strokeWidth={1.4} opacity={0.9} />
          <polyline
            points="105.5,16.5 110,22 114.5,16.5"
            fill="none"
            stroke="currentColor"
            strokeWidth={1.4}
            opacity={0.9}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </g>
        <g style={{ animationDelay: `${INTAKE_STAGGER_MS * 2}ms` }}>
          <line x1={132} y1={6} x2={132} y2={20} stroke="currentColor" strokeWidth={1.25} opacity={0.5} />
          <polyline
            points="128,15 132,20 136,15"
            fill="none"
            stroke="currentColor"
            strokeWidth={1.25}
            opacity={0.5}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </g>
      </g>

      {/* Document — perspective sheet with folded corner */}
      <path
        d="M 56 30 L 140 30 L 158 48 L 158 102 L 56 102 Z"
        fill="none"
        stroke="currentColor"
        strokeWidth={1.5}
        strokeLinejoin="round"
      />
      <path
        d="M 140 30 L 140 48 L 158 48"
        fill="none"
        stroke="currentColor"
        strokeWidth={1.25}
        opacity={0.7}
      />

      {/* Embedded waveform inside the document — the "signal" being absorbed */}
      <g transform="translate(68, 66)" opacity={0.7}>
        {[
          [0, 14], [6, 22], [12, 10], [18, 28], [24, 16],
          [30, 24], [36, 12], [42, 20], [48, 8], [54, 18],
          [60, 14], [66, 22], [72, 10], [78, 16],
        ].map(([dx, h]) => (
          <rect
            key={dx}
            x={dx}
            y={-h / 2}
            width={2}
            height={h}
            fill="currentColor"
            rx={0.5}
          />
        ))}
      </g>

      {/* Unifying terminus waveform — to the right of the doc, smaller */}
      <TerminusWaveform x={178} y={55} opacity={0.7} />
    </svg>
  )
}
