import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { speakerTone } from '@/lib/speaker-tone'
import { cn, formatDuration } from '@/lib/utils'
import type { Utterance } from '@/types/transcription'
import { layoutStrip, seekTimeInSegment, turnAt, type StripLayout } from './speaker-strip'

/** The width the strip lays out for until it has measured itself (and in
 *  jsdom, which never does): about the rail's inner width. Positions are drawn
 *  as percentages, so this only sets how finely turns merge. */
const FALLBACK_WIDTH = 320

/** Speakers named in the screen-reader summary; anyone past this is left out
 *  rather than reading a list nobody could follow. */
const MAX_SUMMARY_SPEAKERS = 10

export interface SpeakerStripProps {
  utterances: Utterance[]
  speakerMap?: Record<string, string>
  /** The audio's duration; 0 while unknown, when the last turn's end stands in. */
  duration: number
  currentTime: number
  onSeek: (time: number) => void
}

/** The end of the last turn, bounded by one pass. */
function transcriptEnd(utterances: Utterance[]): number {
  let end = 0
  for (const utterance of utterances) end = Math.max(end, utterance.end)
  return end
}

/** "Ana Vidal 38 %, Tomas Bergh 22 %", busiest first: what the strip shows,
 *  for a screen reader, which cannot see lane widths. */
function speakingTimeSummary(
  layout: StripLayout,
  nameOf: (speaker: number) => string,
  locale: string,
): string {
  if (layout.totalTalk <= 0) return ''
  const percent = new Intl.NumberFormat(locale, { style: 'percent', maximumFractionDigits: 0 })
  const shares = [...layout.talkTime.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, MAX_SUMMARY_SPEAKERS)
    .map(([speaker, seconds]) => `${nameOf(speaker)} ${percent.format(seconds / layout.totalTalk)}`)
  return shares.join(", ")
}

/** Measures the strip's own width, so turns merge at real pixel resolution. */
function useStripWidth() {
  // A callback ref held in state: the strip renders nothing until it has data,
  // so the node can arrive after the first effect pass.
  const [node, ref] = useState<HTMLDivElement | null>(null)
  const [width, setWidth] = useState(0)
  useEffect(() => {
    if (!node || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver((entries) => {
      const measured = entries[0]?.contentRect.width
      if (measured && measured > 0) setWidth(Math.round(measured))
    })
    observer.observe(node)
    return () => observer.disconnect()
  }, [node])
  return { ref, width: width > 0 ? width : FALLBACK_WIDTH }
}

/**
 * Who spoke when, inside the ink audio card: one 4 px lane per speaker (five
 * at most), each turn a segment in the speaker's disc colour, the playing turn
 * in coral and a playhead through every lane. Clicking a segment seeks to the
 * turn at that point: on a long hearing one segment stands for many turns.
 *
 * Decorative for assistive technology: the transcript list is the accessible
 * version of the same facts, and the slider stays the keyboard control. A
 * screen reader gets the speaking-time shares instead. Nothing moves except
 * the playhead following playback.
 */
export function SpeakerStrip({
  utterances,
  speakerMap,
  duration,
  currentTime,
  onSeek,
}: SpeakerStripProps) {
  const { t, i18n } = useTranslation(['media', 'transcription'])
  const { ref, width } = useStripWidth()
  const span = duration > 0 ? duration : transcriptEnd(utterances)
  const layout = useMemo(() => layoutStrip(utterances, span, width), [utterances, span, width])

  if (layout.lanes.length === 0) return null

  const nameOf = (speaker: number) =>
    speakerMap?.[String(speaker)]?.trim() ||
    t('transcription:speakers.speakerLabel', { index: speaker + 1 })
  const playhead = Math.min(100, Math.max(0, (currentTime / span) * 100))
  const playingTurn = turnAt(utterances, currentTime)
  const summary = speakingTimeSummary(layout, nameOf, i18n.language)

  return (
    <div>
      <div ref={ref} aria-hidden="true" data-testid="speaker-strip" className="relative space-y-[3px]">
        {layout.lanes.map((lane) => (
          <div key={lane.speakers.join('-')} className="relative h-1 rounded-full bg-white/10">
            {lane.segments.map((segment) => (
              <button
                key={segment.start}
                type="button"
                tabIndex={-1}
                title={`${nameOf(segment.speaker)} · ${formatDuration(segment.start)}`}
                onClick={(event) => {
                  const box = event.currentTarget.getBoundingClientRect()
                  const fraction = box.width > 0 ? (event.clientX - box.left) / box.width : 0
                  onSeek(seekTimeInSegment(utterances, lane.speakers, segment, fraction))
                }}
                className={cn(
                  // The hit area reaches 2 px past the 4 px mark on each side.
                  "absolute inset-y-0 rounded-[1px] after:absolute after:inset-x-0 after:-inset-y-0.5 after:content-['']",
                  speakerTone(segment.speaker).fill,
                )}
                style={{ left: `${(segment.x / width) * 100}%`, width: `${(segment.w / width) * 100}%` }}
              />
            ))}
            {/* Coral for the turn itself, not the merged segment around it, and
                never under 2 px, so a 10-second turn in a long hearing shows. */}
            {playingTurn && lane.speakers.includes(playingTurn.speaker) && (
              <span
                data-playing="true"
                className="pointer-events-none absolute -inset-y-px min-w-0.5 rounded-[1px] bg-coral-mark"
                style={{
                  left: `${(playingTurn.start / span) * 100}%`,
                  width: `${((playingTurn.end - playingTurn.start) / span) * 100}%`,
                }}
              />
            )}
          </div>
        ))}
        <span
          className="pointer-events-none absolute -inset-y-0.5 w-px bg-white/80"
          style={{ left: `${playhead}%` }}
        />
      </div>
      {summary && <p className="sr-only">{t('media:player.speakingTime', { shares: summary })}</p>}
    </div>
  )
}
