import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronUp, Pause, Play } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { PILL_CLASS } from '@/components/ui/chip'
import { Slider } from '@/components/ui/slider'
import { SpeedSelect, THIN_SLIDER } from '@/components/media/AudioPlayer'
import { cn, formatDuration } from '@/lib/utils'

/** `THIN_SLIDER` on paper: a hairline secondary track instead of the audio
 *  card's translucent white one. */
const PAPER_SLIDER = `${THIN_SLIDER} [&>span:first-child]:bg-secondary`

export interface MiniDockProps {
  isPlaying: boolean
  currentTime: number
  duration: number
  speed: number
  onToggle: () => void
  onSeek: (time: number) => void
  onSpeedChange: (rate: number) => void
  onBackToTranscript: () => void
}

/**
 * The transport that reappears once the audio card has scrolled away — while
 * the reader is down in the briefing, say. Purely presentational: it drives the
 * page's single `<audio>` element through the callbacks, and owns no state of
 * its own, so nothing here can disagree with the card.
 */
export function MiniDock({
  isPlaying,
  currentTime,
  duration,
  speed,
  onToggle,
  onSeek,
  onSpeedChange,
  onBackToTranscript,
}: MiniDockProps) {
  const { t } = useTranslation(['media', 'transcription'])
  // The scrubber is controlled by a time the page throttles to 4 Hz, so a drag
  // rendered straight from `currentTime` fights the pointer: most pointermove
  // frames are dropped and the handle snaps backwards. While a drag is in
  // flight the dock shows the dragged value and moves nothing; the commit
  // seeks, and playback takes the handle back.
  const [dragTime, setDragTime] = useState<number | null>(null)

  return (
    // z-30 keeps the dock under the floating ActivityTray (z-50, bottom-4
    // right-4, w-80): the 21rem of right padding is that tray plus its inset,
    // and it starts at `md` because that is where the dock stops being full
    // width. The left inset clears MainLayout's sidebar, whose width is a token.
    <div
      role="region"
      aria-label={t('media:player.dock')}
      data-testid="reader-mini-dock"
      className="fixed bottom-0 left-0 right-0 z-30 h-14 border-t bg-background/95 backdrop-blur md:left-[var(--app-sidebar-w)] md:pr-[21rem]"
    >
      <div className="mx-auto flex h-full max-w-[1280px] items-center gap-3 px-4 sm:gap-4 sm:px-6">
        <Button
          variant="outline"
          size="icon"
          className="h-9 w-9 shrink-0 rounded-full"
          onClick={onToggle}
          aria-label={isPlaying ? t('media:player.pause') : t('media:player.play')}
        >
          {isPlaying ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4" />}
        </Button>

        <span className="shrink-0 tabular-nums text-xs text-ink-muted dark:text-muted-foreground">
          {formatDuration(dragTime ?? currentTime)} / {formatDuration(duration)}
        </span>

        <Slider
          value={[dragTime ?? currentTime]}
          max={duration || 100}
          step={0.1}
          onValueChange={(value) => {
            if (value[0] !== undefined) setDragTime(value[0])
          }}
          onValueCommit={(value) => {
            setDragTime(null)
            if (value[0] !== undefined) onSeek(value[0])
          }}
          aria-label={t('media:player.position')}
          className={`min-w-0 flex-1 ${PAPER_SLIDER}`}
        />

        <SpeedSelect
          speed={speed}
          onSpeedChange={onSpeedChange}
          className={cn(PILL_CLASS, 'h-8 w-auto shrink-0 gap-1 px-2.5 text-xs')}
        />

        <Button
          variant="ghost"
          size="sm"
          className="shrink-0 gap-1.5 px-2 text-sm"
          onClick={onBackToTranscript}
          aria-label={t('media:player.backToTranscript')}
        >
          <ChevronUp className="h-4 w-4" aria-hidden="true" />
          <span className="hidden lg:inline">{t('media:player.backToTranscript')}</span>
        </Button>
      </div>
    </div>
  )
}
