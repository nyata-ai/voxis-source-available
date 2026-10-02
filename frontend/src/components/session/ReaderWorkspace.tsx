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
import { AudioAnalysisCard } from '@/components/media/AudioAnalysisCard'
import { SpeakerEditor } from '@/components/transcription/SpeakerEditor'
import { TranscriptViewer } from '@/components/transcription/TranscriptViewer'
import type { SummaryType } from '@/types/summary'
import { ExportPanel } from './ExportPanel'
import type { SessionView } from './session-view'

/** A 36×20 switch — the toolbar's control, not the settings page's. */
const FOLLOW_SWITCH =
  'h-5 w-9 [&>span]:h-4 [&>span]:w-4 [&>span]:data-[state=checked]:translate-x-4'

/** The transcript column pins under the app header from `lg` and scrolls
 *  inside its own box, so the words stay put while the left rail — audio card,
 *  export, speakers, integrity — flows with the page. The 16 px of air under the
 *  header is opaque padding on the sticky element itself, so scrolled content
 *  never shows through the gap. `--app-header-h` is set in globals.css. */
const COLUMN_STICKY = 'lg:sticky lg:top-[var(--app-header-h)] lg:bg-background lg:pt-4'

interface FindState {
  query: string
  matchCount: number
  activeMatch: number
  onQueryChange: (value: string) => void
  onStep: (delta: number) => void
}

/**
 * Find-in-transcript. Enter steps to the next match and wraps, Shift+Enter
 * steps back, Escape clears the query and blurs — none of which touches
 * follow-playback; stepping goes through the viewer's own jump, which releases
 * follow on its own.
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
      <div className="relative flex-1 basis-full sm:basis-60">
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
              ? t('transcription:reader.find.count', {
                  current: activeMatch + 1,
                  total: matchCount,
                })
              : t('transcription:reader.find.none')}
          </span>
          <span className="flex min-h-11 items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7 rounded-lg"
              disabled={matchCount === 0}
              onClick={() => onStep(-1)}
              aria-label={t('transcription:reader.find.prev')}
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7 rounded-lg"
              disabled={matchCount === 0}
              onClick={() => onStep(1)}
              aria-label={t('transcription:reader.find.next')}
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7 rounded-lg"
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
}: {
  session: SessionView
  currentTime: number
  query: string
  activeMatch: number
  following: boolean
  onSeek: (time: number) => void
  onMatchesChange: (matches: number[]) => void
  onFollowingChange: (following: boolean) => void
}) {
  if (session.transcriptCta) return <>{session.transcriptCta}</>

  if (session.utterances.length === 0 && session.fullTranscript) {
    return (
      <pre className="whitespace-pre-wrap text-[15px] leading-[1.65]">{session.fullTranscript}</pre>
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
      {showsSpeakers(session) && session.transcriptionId && (
        <SpeakerEditor
          transcriptionId={session.transcriptionId}
          speakerMap={session.speakerMap ?? {}}
          speakerCount={session.speakerCount}
          suggestedSpeakerMap={session.suggestedSpeakerMap}
          suggestionsGenerated={session.suggestionsGenerated}
        />
      )}
      {session.forensics && (
        <AudioAnalysisCard
          analysis={session.forensics.analysis}
          fileHash={session.forensics.fileHash}
        />
      )}
    </>
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
}

/**
 * The reading surface: a narrow left rail of things you act on — player,
 * export, speakers, provenance — beside the transcript, which gets the wider
 * column and its own scroll. Both rails stick under the top bar from `lg` up,
 * so the transport never scrolls away from the words.
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
}: ReaderWorkspaceProps) {
  const { t } = useTranslation(['transcription'])
  const [query, setQuery] = useState('')
  const [matches, setMatches] = useState<number[]>([])
  const [activeMatch, setActiveMatch] = useState(0)
  const [following, setFollowing] = useState(true)

  const handleMatchesChange = useCallback((next: number[]) => {
    setMatches(next)
    setActiveMatch(0)
  }, [])

  const stepMatch = useCallback(
    (delta: number) => {
      setActiveMatch((current) => {
        const total = matches.length
        if (total === 0) return 0
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
      className="grid items-start gap-8 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]"
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
              {showExport && (
                <ExportPanel session={session} activeLens={activeLens} bapEligible={bapEligible} />
              )}
              <ReaderFilePanels session={session} />
            </div>
          )}
        </div>
      </div>

      <div ref={columnRef} className={`order-2 min-w-0 ${COLUMN_STICKY}`}>
        {hasTranscriptText && (
          <div className="flex flex-wrap items-center gap-3 pb-3">
            <FindField
              query={query}
              matchCount={matches.length}
              activeMatch={activeMatch}
              onQueryChange={setQuery}
              onStep={stepMatch}
            />
            <span className="inline-flex min-h-11 items-center gap-2">
              <Switch
                id="reader-follow"
                checked={following}
                onCheckedChange={setFollowing}
                className={FOLLOW_SWITCH}
              />
              <Label htmlFor="reader-follow" className="cursor-pointer text-[13px] font-normal">
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
        />
      </div>
    </div>
  )
}
