import {
  useCallback,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
  type Ref,
} from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronLeft, ChevronRight, Search, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { AudioAnalysisCard } from '@/components/media/AudioAnalysisCard'
import { SpeakerEditor } from '@/components/transcription/SpeakerEditor'
import {
  TranscriptViewer,
  type CitationLanding,
} from '@/components/transcription/TranscriptViewer'
import type { SummaryType } from '@/types/summary'
import { ExportPanel } from './ExportPanel'
import type { SessionView } from './session-view'

/** A 36×20 switch — the toolbar's control, not the settings page's. */
const FOLLOW_SWITCH =
  'h-5 w-9 [&>span]:h-4 [&>span]:w-4 [&>span]:data-[state=checked]:translate-x-4'

/** The transcript column pins under the app header (and the reader's section
 *  links, when the page shows them) from `lg` and scrolls inside its own box,
 *  so the words stay put while the left rail — audio card, speakers, export,
 *  integrity — flows with the page. The 16 px of air under the header is opaque
 *  padding on the sticky element itself, so scrolled content never shows
 *  through the gap. `--app-header-h` is set in globals.css; `--reader-nav-h` by
 *  the reader page. */
const COLUMN_STICKY =
  'lg:sticky lg:top-[calc(var(--app-header-h)_+_var(--reader-nav-h,0rem))] lg:bg-background lg:pt-4'

/** From `lg` the find row rides with the sticky column. Below it the page is the
 *  one scroll, so the row pins itself under the same bars, opaque, or stepping
 *  through matches would carry the Next button off the screen. The auto-scroll
 *  hook finds it by `data-reader-find` to land turns below it. */
const FIND_ROW_STICKY =
  'max-lg:sticky max-lg:top-[calc(var(--app-header-h)_+_var(--reader-nav-h,0rem))] max-lg:z-10 max-lg:bg-background max-lg:pt-2'

/** 44 px below `sm`, the toolbar's quieter 28 px above it. */
const FIND_BUTTON = 'h-11 w-11 rounded-lg sm:h-7 sm:w-7'

/** Where an in-page section link lands: under the app header and the sticky
 *  section links, with a little air. */
export const READER_SECTION_SCROLL_MARGIN =
  'scroll-mt-[calc(var(--app-header-h)_+_var(--reader-nav-h,0rem)_+_0.5rem)]'

/** DOM ids of the reader's sections, for the section links. */
export const READER_SECTION_ID = {
  transcript: 'transcript',
  export: 'export',
} as const

interface FindState {
  query: string
  matchCount: number
  activeMatch: number
  onQueryChange: (value: string) => void
  onStep: (delta: number) => void
}

/**
 * Find-in-transcript. Enter goes to the first match, then steps and wraps,
 * Shift+Enter steps back, Escape clears the query and blurs — none of which
 * touches follow-playback; stepping goes through the viewer's own jump, which
 * releases follow on its own.
 */
function FindField({ query, matchCount, activeMatch, onQueryChange, onStep }: FindState) {
  const { t } = useTranslation(['transcription'])
  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLInputElement>) => {
      // Enter/Escape during IME composition confirm or cancel the candidate
      // (ja/zh/ko input) — they must never step matches or clear the query.
      if (event.nativeEvent.isComposing) return
      if (event.key === 'Enter') {
        event.preventDefault()
        onStep(event.shiftKey ? -1 : 1)
        return
      }
      if (event.key === 'Escape') {
        event.preventDefault()
        onQueryChange('')
        event.currentTarget.blur()
      }
    },
    [onQueryChange, onStep]
  )

  const isSearching = query.trim() !== ''

  return (
    <>
      <div className="relative min-w-24 flex-1 basis-0 sm:basis-60">
        <Search
          className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          type="text"
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          onKeyDown={handleKeyDown}
          aria-label={t('transcription:reader.find.label')}
          placeholder={t('transcription:reader.find.placeholder')}
          className="h-11 rounded-xl pl-10 text-sm sm:h-10"
        />
      </div>
      {isSearching && (
        <>
          <span
            data-testid="reader-find-count"
            role="status"
            className="shrink-0 tabular-nums text-xs text-ink-muted dark:text-muted-foreground"
          >
            {matchCount > 0
              ? t('transcription:reader.find.count', { current: Math.max(activeMatch, 0) + 1, total: matchCount })
              : t('transcription:reader.find.none')}
          </span>
          <span className="flex min-h-11 items-center sm:gap-1">
            <Button
              variant="ghost"
              size="icon"
              className={FIND_BUTTON}
              disabled={matchCount === 0}
              onClick={() => onStep(-1)}
              aria-label={t('transcription:reader.find.prev')}
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className={FIND_BUTTON}
              disabled={matchCount === 0}
              onClick={() => onStep(1)}
              aria-label={t('transcription:reader.find.next')}
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className={FIND_BUTTON}
              onClick={() => onQueryChange('')}
              aria-label={t('transcription:reader.find.clear')}
            >
              <X className="h-4 w-4" />
            </Button>
          </span>
        </>
      )}
    </>
  )
}

/**
 * The transcript column, in order of what the session actually has: the
 * adapter's call to action when there is no transcript at all, a plain-text
 * fallback when there is text but no segments, and the viewer otherwise.
 */
function TranscriptBody({
  session,
  currentTime,
  query,
  activeMatch,
  following,
  onSeek,
  onMatchesChange,
  onFollowingChange,
  landing,
  isPlaying,
}: {
  session: SessionView
  currentTime: number
  query: string
  activeMatch: number
  following: boolean
  onSeek: (time: number) => void
  onMatchesChange: (matches: number[]) => void
  onFollowingChange: (following: boolean) => void
  landing?: CitationLanding | null
  isPlaying?: boolean
}) {
  if (session.transcriptCta) return <>{session.transcriptCta}</>

  if (session.utterances.length === 0 && session.fullTranscript) {
    return (
      <pre className="max-w-[68ch] whitespace-pre-wrap font-sans text-reading">{session.fullTranscript}</pre>
    )
  }

  return (
    <TranscriptViewer
      utterances={session.utterances}
      currentTime={currentTime}
      speakerMap={session.speakerMap}
      onSeek={onSeek}
      searchQuery={query}
      activeMatch={activeMatch}
      onMatchesChange={onMatchesChange}
      // B contract, see plan C1: the viewer's follow state is controlled here
      // so the toolbar switch and the floating pill cannot disagree.
      following={following}
      onFollowingChange={onFollowingChange}
      landing={landing}
      isPlaying={isPlaying}
    />
  )
}

/** The mini dock's height, handed to the rail as a custom property so its
 *  scroll stops above the dock rather than underneath it. */
const DOCK_UP = { '--reader-dock-h': '3.5rem' } as CSSProperties

/** Speakers are editable only when there are speakers and a transcription to
 *  attach the names to. */
function showsSpeakers(session: SessionView): boolean {
  return session.can.speakers && session.speakerCount > 0 && Boolean(session.transcriptionId)
}

function hasFilePanels(session: SessionView): boolean {
  return showsSpeakers(session) || Boolean(session.forensics)
}

/**
 * Speakers and provenance: the two panels that describe the FILE rather than
 * the transcript. Exported because the page renders them even when it withholds
 * the workspace — a failed or still-running transcription is exactly when
 * someone wants the SHA-256 and the integrity checks. Renders nothing when the
 * session has neither.
 */
export function ReaderFilePanels({ session }: { session: SessionView }) {
  if (!hasFilePanels(session)) return null
  return (
    <>
      <SpeakersPanel session={session} />
      <AnalysisPanel session={session} />
    </>
  )
}

function SpeakersPanel({ session }: { session: SessionView }) {
  if (!showsSpeakers(session) || !session.transcriptionId) return null
  return (
    <SpeakerEditor
      transcriptionId={session.transcriptionId}
      speakerMap={session.speakerMap ?? {}}
      speakerCount={session.speakerCount}
      suggestedSpeakerMap={session.suggestedSpeakerMap}
      suggestionsGenerated={session.suggestionsGenerated}
    />
  )
}

function AnalysisPanel({ session }: { session: SessionView }) {
  if (!session.forensics) return null
  return (
    <AudioAnalysisCard
      analysis={session.forensics.analysis}
      fileHash={session.forensics.fileHash}
    />
  )
}

export interface ReaderWorkspaceProps {
  session: SessionView
  currentTime: number
  onSeek: (time: number) => void
  /** The briefing tab on screen — the export panel's Briefing row follows it. */
  activeLens: SummaryType
  bapEligible: boolean
  /** The audio card, built by the page because it owns the player handle. */
  audioCard?: ReactNode
  /** Goes on the transcript column: the mini dock scrolls back to it. */
  columnRef?: Ref<HTMLDivElement>
  /** Whether the mini dock is covering the bottom of the viewport, so the rail
   *  can stop its scroll above it instead of underneath it. */
  dockVisible?: boolean
  /** A briefing citation landing on its transcript turn. */
  landing?: CitationLanding | null
  /** Whether the audio is playing: below `lg`, follow only lets go of a page
   *  scrolled by hand while it is. */
  isPlaying?: boolean
}

/**
 * The reading surface: a fixed rail (300-360 px) of things you act on, in the
 * order you use them — player, speakers, export, provenance — beside the
 * transcript, which takes every pixel left over and its own scroll. The
 * transcript sticks under the top bar from `lg` up, so the words stay put.
 */
export function ReaderWorkspace({
  session,
  currentTime,
  onSeek,
  activeLens,
  bapEligible,
  audioCard,
  columnRef,
  dockVisible = false,
  landing,
  isPlaying,
}: ReaderWorkspaceProps) {
  const { t } = useTranslation(['transcription'])
  const [query, setQuery] = useState('')
  const [matches, setMatches] = useState<number[]>([])
  // -1 until the reader steps, so the first Enter goes to match 1 rather than 2.
  const [activeMatch, setActiveMatch] = useState(-1)
  const [following, setFollowing] = useState(true)

  const handleMatchesChange = useCallback((next: number[]) => {
    setMatches(next)
    setActiveMatch(-1)
  }, [])

  const stepMatch = useCallback(
    (delta: number) => {
      setActiveMatch((current) => {
        const total = matches.length
        if (total === 0) return -1
        if (current < 0) return delta > 0 ? 0 : total - 1
        return (current + delta + total) % total
      })
    },
    [matches.length]
  )

  // Find needs something to search: an untranscribed session's column holds a
  // call to action, not text.
  const hasTranscriptText = session.utterances.length > 0 || Boolean(session.fullTranscript)
  const showExport = session.can.export || session.can.downloadAudio
  const showAudioBlock = Boolean(audioCard)
  const showPanels = showExport || hasFilePanels(session)

  return (
    <div
      className="grid items-start gap-8 lg:grid-cols-[minmax(300px,360px)_minmax(0,1fr)]"
      style={dockVisible ? DOCK_UP : undefined}
    >
      {/* The rail is one column from `lg` and flows with the page — no inner
          scroll region, so no second scrollbar over the panels; when it is the
          taller column the transcript beside it stays pinned. Below `lg` the
          wrapper dissolves (`display: contents`) so its two halves become grid
          items in their own right and `order` can put the transcript between
          them, as the phone artboard reads: audio card, transcript, panels. */}
      <div className="contents lg:block lg:space-y-6 lg:pt-4">
        <div className="contents lg:block lg:space-y-6">
          {showAudioBlock && <div className="order-1 space-y-6">{audioCard}</div>}
          {showPanels && (
            <div className="order-3 space-y-6">
              <SpeakersPanel session={session} />
              {showExport && (
                <div id={READER_SECTION_ID.export} className={READER_SECTION_SCROLL_MARGIN}>
                  <ExportPanel session={session} activeLens={activeLens} bapEligible={bapEligible} />
                </div>
              )}
              <AnalysisPanel session={session} />
            </div>
          )}
        </div>
      </div>

      <div
        ref={columnRef}
        id={READER_SECTION_ID.transcript}
        className={`order-2 min-w-0 ${COLUMN_STICKY} ${READER_SECTION_SCROLL_MARGIN}`}
      >
        {hasTranscriptText && (
          <div
            data-reader-find=""
            className={cn('flex flex-wrap items-center gap-2 pb-2 sm:gap-3 sm:pb-3', FIND_ROW_STICKY)}
          >
            <FindField
              query={query}
              matchCount={matches.length}
              activeMatch={activeMatch}
              onQueryChange={setQuery}
              onStep={stepMatch}
            />
            {/* A phone has one row for find. While it is searching, the switch
                steps aside: stepping releases follow, and the Follow pill is
                the way back. */}
            <span className={cn('inline-flex min-h-11 items-center gap-2', query.trim() !== '' && 'max-sm:hidden')}>
              <Switch
                id="reader-follow"
                checked={following}
                onCheckedChange={setFollowing}
                className={FOLLOW_SWITCH}
              />
              <Label htmlFor="reader-follow" className="cursor-pointer text-body font-normal">
                {t('transcription:reader.followPlayback')}
              </Label>
            </span>
          </div>
        )}
        <TranscriptBody
          session={session}
          currentTime={currentTime}
          query={query}
          activeMatch={activeMatch}
          following={following}
          onSeek={onSeek}
          onMatchesChange={handleMatchesChange}
          onFollowingChange={setFollowing}
          landing={landing}
          isPlaying={isPlaying}
        />
      </div>
    </div>
  )
}
