import { memo, useCallback, useEffect, useMemo, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowDown } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { formatDuration } from '@/lib/utils'
import { cn } from '@/lib/utils'
import { useTranscriptAutoScroll } from '@/hooks/useTranscriptAutoScroll'
import type { TFunction } from 'i18next'
import type { Utterance } from '@/types/transcription'

/**
 * Upper bound on the `<mark>` runs rendered for one utterance. A one-character
 * query against a long segment would otherwise build a node per character.
 * Past this point the tail renders unmarked — the row is still flagged as a
 * match, which is what the reader navigates by.
 */
const MAX_MARKS_PER_ROW = 50

interface TranscriptViewerProps {
  utterances: Utterance[]
  currentTime?: number
  speakerMap?: Record<string, string>
  /** Seeks playback to a turn's start. Passed straight to the rows: a click is
   *  a seek and nothing more. It deliberately does NOT re-engage `following` —
   *  that is a switch the reader set, and re-engaging would additionally scroll
   *  to the turn that was playing before the seek, not the one just clicked. */
  onSeek?: (time: number) => void
  /** Find-in-transcript needle. Matching rows are marked, non-matching rows are
   *  dimmed — never removed. Rows are addressed by array index (auto-scroll
   *  refs, React keys), so filtering them out would misaddress every segment
   *  after the first hidden one. */
  searchQuery?: string
  /** Ordinal into the reported match list (not an utterance index): 0 is the
   *  first match. Changing it scrolls that match into view. */
  activeMatch?: number
  /** Reports the matching utterance indices, in document order, whenever the
   *  query or the utterances change. Pass a stable (memoized) callback. */
  onMatchesChange?: (matches: number[]) => void
  /** Controlled: true while the list follows playback. The reader's toolbar
   *  switch and this component's Follow pill are two faces of one state, so it
   *  lives above both rather than inside either. */
  following: boolean
  /** Called with `false` when the reader scrolls away from playback (or steps
   *  to a search match) and with `true` when the Follow pill is pressed. */
  onFollowingChange: (following: boolean) => void
}

/** Splits `text` into plain runs and `<mark>` runs on a case-insensitive
 *  needle. Returns the raw string when nothing matches, so unmatched rows
 *  render exactly as they did before find existed. */
function markMatches(text: string, needle: string): ReactNode {
  if (!needle) return text
  const haystack = text.toLowerCase()
  // Indices into the lowercased copy only address the original when lowercasing
  // preserved every character's position. It does not always: `İ` (U+0130)
  // lowercases to two code units, which would shift every slice after it and
  // render mangled words. Unicode lowercase mappings never contract, so equal
  // lengths mean equal positions — and when they differ, the row is still
  // reported as a match and navigable, it just renders unmarked.
  if (haystack.length !== text.length) return text
  const parts: ReactNode[] = []
  let cursor = 0

  for (let run = 0; run < MAX_MARKS_PER_ROW; run++) {
    const at = haystack.indexOf(needle, cursor)
    if (at === -1) break
    if (at > cursor) parts.push(text.slice(cursor, at))
    parts.push(
      <mark
        key={at}
        className="rounded-[3px] bg-coral/20 px-0.5 text-foreground dark:bg-coral/40"
      >
        {text.slice(at, at + needle.length)}
      </mark>
    )
    cursor = at + needle.length
  }

  if (parts.length === 0) return text
  if (cursor < text.length) parts.push(text.slice(cursor))
  return parts
}

interface UtteranceRowProps {
  index: number
  utterance: Utterance
  isActive: boolean
  speakerLabel: string
  onSeek?: (time: number) => void
  registerRef: (id: number, el: HTMLElement | null) => void
  t: TFunction<'transcription'>
  /** Normalized (lowercased, trimmed) needle; empty when find is idle. */
  needle: string
  isMatch: boolean
  isActiveMatch: boolean
}

const UtteranceRow = memo(function UtteranceRow({
  index,
  utterance,
  isActive,
  speakerLabel,
  onSeek,
  registerRef,
  t,
  needle,
  isMatch,
  isActiveMatch,
}: UtteranceRowProps) {
  const setRef = useCallback(
    (el: HTMLButtonElement | null) => registerRef(index, el),
    [index, registerRef]
  )

  const isSearching = needle !== ''
  // A const alias so the narrowing survives into the JSX branch below — the
  // score is only ever read where it is known to exist.
  const confidence = utterance.confidence
  const isLowConfidence = confidence !== undefined && confidence < 0.8

  // `role="listitem"` rides a wrapper, never the button: on the button it would
  // replace the button role, and a screen reader would lose the one thing that
  // says the turn can be pressed to seek. The wrapper is layout-free — the
  // button still draws the whole row — and carries the find-state hooks.
  return (
    <div
      role="listitem"
      data-match={isSearching ? String(isMatch) : undefined}
      data-active-match={isActiveMatch ? 'true' : undefined}
    >
      <button
        ref={setRef}
        type="button"
        onClick={() => onSeek?.(utterance.start)}
        className={cn(
          // The reading rail: a fixed 96 px meta column beside the words on a
          // tablet and up, stacked on a phone where 96 px would halve the line.
          // Borderless: rows are separated by air, not hairlines, and the
          // playing turn is a soft tint with a coral edge drawn as an inset
          // shadow, so nothing shifts when it lights up.
          'grid w-full grid-cols-1 gap-1 rounded-lg px-3 py-3.5 text-left',
          'sm:grid-cols-[96px_minmax(0,1fr)] sm:gap-4',
          'transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
          isActive
            ? 'bg-paper-soft shadow-[inset_3px_0_0_var(--color-coral)] dark:bg-muted'
            : 'hover:bg-paper-soft/70 dark:hover:bg-muted/50',
          // Dim, never remove: the row stays addressable by its index.
          isSearching && !isMatch && 'opacity-40',
          isActiveMatch && 'ring-2 ring-coral/50'
        )}
      >
        <span className="flex flex-wrap items-baseline gap-x-2.5 gap-y-0.5 sm:flex-col sm:items-start">
          <span className="text-xs font-semibold text-foreground">{speakerLabel}</span>
          <span className="text-xs text-ink-muted dark:text-muted-foreground">
            {formatDuration(utterance.start)}
          </span>
          {isLowConfidence && (
            <span
              className="text-[11px] text-warning"
              title={t('viewer.confidenceTitle', { percent: Math.round(confidence * 100) })}
            >
              {t('viewer.lowConfidence')}
            </span>
          )}
        </span>
        <p className="min-w-0 text-[15px] leading-[1.65] text-foreground">
          {isMatch ? markMatches(utterance.text, needle) : utterance.text}
        </p>
      </button>
    </div>
  )
})

export function TranscriptViewer({
  utterances,
  currentTime,
  speakerMap,
  onSeek,
  searchQuery,
  activeMatch,
  onMatchesChange,
  following,
  onFollowingChange,
}: TranscriptViewerProps) {
  const { t } = useTranslation('transcription')
  const scrollContainerRef = useRef<HTMLDivElement>(null)

  const needle = searchQuery?.trim().toLowerCase() ?? ''

  // Lowercase once per transcript, not once per keystroke — long transcripts
  // make the per-keystroke pass an O(n) string-allocation sweep otherwise.
  const lowercasedTexts = useMemo(() => utterances.map((u) => u.text.toLowerCase()), [utterances])

  const matches = useMemo(() => {
    if (needle === '') return []
    const found: number[] = []
    for (let i = 0; i < lowercasedTexts.length; i++) {
      if (lowercasedTexts[i]!.indexOf(needle) !== -1) found.push(i)
    }
    return found
  }, [needle, lowercasedTexts])

  const activeIndex =
    currentTime !== undefined
      ? utterances.findIndex((u) => currentTime >= u.start && currentTime < u.end)
      : -1

  const { followPlayback, jumpToSegment, registerSegmentRef } = useTranscriptAutoScroll({
    containerRef: scrollContainerRef,
    activeSegmentId: activeIndex >= 0 ? activeIndex : null,
    // The scroll box only exists once there is something to scroll; the hook
    // needs to know so it can subscribe when the container finally appears.
    segmentCount: utterances.length,
    isFollowing: following,
    onFollowingChange,
  })

  useEffect(() => {
    onMatchesChange?.(matches)
  }, [matches, onMatchesChange])

  // The active match owns the viewport while the reader is *stepping*; jumping
  // releases playback follow so the next time update cannot pull it away.
  //
  // Only a step counts. The reader shell resets `activeMatch` to 0 on every
  // match-list change, so a jump keyed on the resolved index alone fired on the
  // first character of a query — silently switching "Follow playback" off, with
  // nothing to switch it back on when the query was cleared. An `activeMatch`
  // that moved while the needle stood still is the reader stepping; anything
  // else is the query changing under it.
  const activeMatchIndex = activeMatch !== undefined ? matches[activeMatch] : undefined
  const lastStep = useRef({ needle, activeMatch })
  useEffect(() => {
    const previous = lastStep.current
    lastStep.current = { needle, activeMatch }
    if (previous.needle !== needle) return
    if (previous.activeMatch === activeMatch) return
    if (activeMatchIndex === undefined) return
    jumpToSegment(activeMatchIndex)
  }, [needle, activeMatch, activeMatchIndex, jumpToSegment])

  if (!utterances.length) {
    return <p className="text-sm text-muted-foreground py-4">{t('viewer.noUtterances')}</p>
  }

  const matchSet = new Set(matches)

  return (
    <div className="relative">
      <div
        ref={scrollContainerRef}
        data-testid="transcript-scroll-container"
        // The gutter is reserved so the words never sit under the scrollbar that
        // appears on hover; the dock height is subtracted while the mini dock is up.
        className="max-h-[60vh] overflow-y-auto overscroll-contain pr-2 [scrollbar-gutter:stable] lg:max-h-[calc(100vh_-_10rem_-_var(--reader-dock-h,0rem))]"
      >
        <div role="list" aria-label={t('viewer.ariaLabel')} className="space-y-0.5">
          {utterances.map((utterance, index) => {
            const isActive = activeIndex === index
            const speakerLabel =
              speakerMap?.[String(utterance.speaker)]?.trim() ||
              t('speakers.speakerLabel', { index: utterance.speaker + 1 })

            return (
              <UtteranceRow
                key={index}
                index={index}
                utterance={utterance}
                isActive={isActive}
                speakerLabel={speakerLabel}
                onSeek={onSeek}
                registerRef={registerSegmentRef}
                t={t}
                needle={needle}
                isMatch={matchSet.has(index)}
                isActiveMatch={activeMatchIndex === index}
              />
            )
          })}
        </div>
      </div>

      {!following && (
        <Button
          variant="secondary"
          size="sm"
          className="absolute bottom-3 right-3 min-h-11 gap-1.5 shadow-md sm:min-h-9"
          onClick={followPlayback}
          aria-label={t('viewer.followLabel')}
        >
          <ArrowDown className="h-3.5 w-3.5" />
          {t('viewer.follow')}
        </Button>
      )}
    </div>
  )
}
