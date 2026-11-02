import { memo, useCallback, useEffect, useMemo, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowDown } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { formatDuration } from '@/lib/utils'
import { cn } from '@/lib/utils'
import { useTranscriptAutoScroll } from '@/hooks/useTranscriptAutoScroll'
import type { TFunction } from 'i18next'
import type { Utterance } from '@/types/transcription'
import { SpeakerDisc } from './SpeakerDisc'
import { buildTurnMeta, findTurnAt, htmlLang, type TurnMeta } from './turn-meta'
import { useReaderScrollMode } from './useReaderScrollMode'

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
   *  first match. Changing it scrolls that match into view. -1 means the reader
   *  has not stepped yet: the first match is flagged, and nothing scrolls. */
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
  /** A citation landing: the turn at `time` is brought on screen and ringed
   *  while this is set. `nonce` restarts the ring when the same citation is
   *  clicked twice; the owner clears the value after the ring's 1.2 s. */
  landing?: CitationLanding | null
  /** Whether the audio is playing; see the auto-scroll hook's window mode. */
  isPlaying?: boolean
}

export interface CitationLanding {
  time: number
  nonce: number
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
  /** The custom name, when one is set: the disc shows its first character. */
  speakerName: string
  /** First turn of a run by this speaker: name and disc show, with air above. */
  showSpeaker: boolean
  /** "ID → EN", when this turn's language tag differs from the turn before. */
  languageSwitchText?: string
  languageSwitchLabel?: string
  /** Set while a citation is landing on this turn; keys the ring. */
  landingNonce?: number
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
  speakerName,
  showSpeaker,
  languageSwitchText,
  languageSwitchLabel,
  landingNonce,
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
      // A new speaker gets 12 px of extra air above the turn: the transcript
      // reads like a script, and the gap is where the eye finds the change.
      className={showSpeaker && index > 0 ? 'pt-3' : undefined}
    >
      <button
        ref={setRef}
        type="button"
        onClick={() => onSeek?.(utterance.start)}
        className={cn(
          // The reading rail: a fixed 112 px meta column beside the words on a
          // tablet and up, stacked on a phone where 112 px would halve the line.
          // Borderless: rows are separated by air, not hairlines, and the
          // playing turn is a soft tint with one straight coral edge, a 3 px
          // bar inset from the corners (a shadow or border would bend round
          // them), so nothing shifts when it lights up.
          'relative grid w-full grid-cols-1 gap-1 rounded-lg px-3 py-3.5 text-left',
          'sm:grid-cols-[112px_minmax(0,1fr)] sm:gap-4',
          'transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
          isActive
            ? "bg-paper-soft before:absolute before:inset-y-2 before:left-0 before:w-[3px] before:rounded-full before:bg-coral-mark before:content-[''] dark:bg-muted"
            : 'hover:bg-paper-soft/70 dark:hover:bg-muted/50',
          // Dim, never remove: the row stays addressable by its index.
          isSearching && !isMatch && 'opacity-40',
          isActiveMatch && 'ring-2 ring-coral-mark/70'
        )}
      >
        {landingNonce !== undefined && (
          <span key={landingNonce} aria-hidden="true" className="citation-landing" />
        )}
        <span className="flex flex-wrap items-center gap-x-2.5 gap-y-0.5 sm:flex-col sm:items-start sm:gap-y-1">
          {showSpeaker ? (
            <span className="flex min-w-0 items-center gap-1.5">
              <SpeakerDisc speakerIndex={utterance.speaker} name={speakerName} />
              <span className="min-w-0 break-words text-body font-semibold text-foreground">
                {speakerLabel}
              </span>
            </span>
          ) : (
            // A follow-on turn hides the name from sight, not from a screen
            // reader: each turn is its own button and must say who speaks.
            <span className="sr-only">{speakerLabel}</span>
          )}
          <span className="text-caption tabular-nums text-ink-muted dark:text-muted-foreground">
            {formatDuration(utterance.start)}
          </span>
          {languageSwitchText && (
            <span className="text-caption font-semibold text-info" title={languageSwitchLabel}>
              <span aria-hidden="true">{languageSwitchText}</span>
              <span className="sr-only">{languageSwitchLabel}</span>
            </span>
          )}
          {isLowConfidence && (
            <span
              className="text-caption text-warning"
              title={t('viewer.confidenceTitle', { percent: Math.round(confidence * 100) })}
            >
              {t('viewer.lowConfidence')}
            </span>
          )}
        </span>
        <p
          lang={htmlLang(utterance.language)}
          className="min-w-0 max-w-[68ch] text-reading text-foreground"
        >
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
  landing,
  isPlaying,
}: TranscriptViewerProps) {
  const { t, i18n } = useTranslation('transcription')
  const scrollContainerRef = useRef<HTMLDivElement>(null)
  const scrollMode = useReaderScrollMode()

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

  const { followPlayback, jumpToSegment, revealSegment, registerSegmentRef } =
    useTranscriptAutoScroll({
      containerRef: scrollContainerRef,
      activeSegmentId: activeIndex >= 0 ? activeIndex : null,
      // The scroll box only exists once there is something to scroll; the hook
      // needs to know so it can subscribe when the container finally appears.
      segmentCount: utterances.length,
      isFollowing: following,
      onFollowingChange,
      scrollMode,
      isPlaying,
    })

  // Speaker runs and language switches, once per transcript (and per UI
  // language, which names the languages for a screen reader).
  const turnMeta = useMemo(
    () => buildTurnMeta(utterances, t, i18n.language),
    [utterances, t, i18n.language],
  )

  // A citation lands: bring its turn on screen. Keyed on the nonce so a
  // second click on the same citation lands again, and so the index settling
  // under playback cannot scroll a second time.
  const landingIndex = landing ? findTurnAt(utterances, landing.time) : -1
  const landingNonce = landing?.nonce
  const landedNonce = useRef<number | undefined>(undefined)
  useEffect(() => {
    if (landingNonce === undefined || landingIndex < 0) return
    if (landedNonce.current === landingNonce) return
    landedNonce.current = landingNonce
    revealSegment(landingIndex)
  }, [landingNonce, landingIndex, revealSegment])

  useEffect(() => {
    onMatchesChange?.(matches)
  }, [matches, onMatchesChange])

  // The active match owns the viewport while the reader is *stepping*; jumping
  // releases playback follow so the next time update cannot pull it away.
  //
  // Only a step counts. The reader shell resets `activeMatch` to -1 on every
  // match-list change, so a jump keyed on the resolved index alone fired on the
  // first character of a query — silently switching "Follow playback" off, with
  // nothing to switch it back on when the query was cleared. An `activeMatch`
  // that moved to a real match while the needle stood still is the reader
  // stepping; anything else is the query changing under it.
  const activeMatchIndex = activeMatch !== undefined ? matches[Math.max(activeMatch, 0)] : undefined
  const lastStep = useRef({ needle, activeMatch })
  useEffect(() => {
    const previous = lastStep.current
    lastStep.current = { needle, activeMatch }
    if (previous.needle !== needle) return
    if (previous.activeMatch === activeMatch) return
    if (activeMatch === undefined || activeMatch < 0) return
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
        data-scroll-mode={scrollMode}
        // From `lg` the transcript scrolls in its own box beside the rail; the
        // gutter is reserved so the words never sit under the scrollbar that
        // appears on hover, and the section-link and dock heights are
        // subtracted. Below `lg` there is no box: the page is the one scroll.
        className="lg:max-h-[calc(100vh_-_10rem_-_var(--reader-nav-h,0rem)_-_var(--reader-dock-h,0rem))] lg:overflow-y-auto lg:overscroll-contain lg:pr-2 lg:[scrollbar-gutter:stable]"
      >
        <div role="list" aria-label={t('viewer.ariaLabel')} className="space-y-0.5">
          {utterances.map((utterance, index) => {
            const isActive = activeIndex === index
            const speakerName = speakerMap?.[String(utterance.speaker)]?.trim() ?? ''
            const speakerLabel =
              speakerName || t('speakers.speakerLabel', { index: utterance.speaker + 1 })
            const meta: TurnMeta | undefined = turnMeta[index]

            return (
              <UtteranceRow
                key={index}
                index={index}
                utterance={utterance}
                isActive={isActive}
                speakerLabel={speakerLabel}
                speakerName={speakerName}
                showSpeaker={meta?.showSpeaker ?? true}
                languageSwitchText={meta?.switchText}
                languageSwitchLabel={meta?.switchLabel}
                landingNonce={landingIndex === index ? landingNonce : undefined}
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
          // Below `lg` the page scrolls, so the pill is pinned to the viewport,
          // centred above the mini dock. On phones the tray's pill shares that
          // band and its text can run to two lines, so while the tray is on
          // screen this one rises clear of that (80 px against 54 px + 16 px).
          // From `md` to `lg` the tray is a 320 px card at the right, and the
          // centre of a 768 px window reaches it, so the pill sits at the left
          // of the content instead.
          className="fixed bottom-[calc(var(--reader-dock-h,0rem)_+_1rem)] left-1/2 z-20 min-h-11 -translate-x-1/2 gap-1.5 shadow-md md:max-lg:left-[calc(var(--app-sidebar-w)_+_1.5rem)] md:max-lg:translate-x-0 max-md:[body:has([data-activity-tray])_&]:bottom-[calc(var(--reader-dock-h,0rem)_+_5rem)] lg:absolute lg:bottom-3 lg:left-auto lg:right-3 lg:min-h-9 lg:translate-x-0"
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
