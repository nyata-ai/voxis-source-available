import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { ArrowLeft, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { DeleteConfirmDialog } from '@/components/media/DeleteConfirmDialog'
import { RetranscribeDialog } from '@/components/transcription/RetranscribeDialog'
import { SUMMARY_TYPE_ORDER } from '@/components/summary/SummaryTypeMeta'
import { useFeatures } from '@/hooks/useFeatures'
import { usePreferences } from '@/hooks/useSettings'
import { normalizeSummaryProfile } from '@/types/settings'
import type { SummaryType } from '@/types/summary'
import { AudioCard } from './AudioCard'
import { BriefingPane } from './BriefingPane'
import { MiniDock } from './MiniDock'
import { ReaderHeader } from './ReaderHeader'
import { ReaderFilePanels, ReaderWorkspace } from './ReaderWorkspace'
import { useReaderPlayback } from './useReaderPlayback'
import type { SessionAudio, SessionLens, SessionView } from './session-view'

/** Statuses where there is nothing to read yet. */
const IN_FLIGHT_STATUSES = new Set(['pending', 'submitted', 'processing'])

/** What each non-playable audio state says. */
const AUDIO_STATE_MESSAGE: Record<SessionAudio['state'], string | null> = {
  available: null,
  unavailable: null,
  deleted: 'media:detail.audioDeleted',
  scan_pending: 'media:detail.scanPending',
  scan_blocked: 'media:detail.scanFailed',
  failed: 'media:detail.streamFailed',
}

/** The DOM id of the briefing band, so `?lens=` deep links can scroll to it. */
const BRIEFING_ANCHOR = 'briefing'

export interface SessionReaderPageProps {
  session: SessionView
}

function isSummaryType(value: string | null | undefined): value is SummaryType {
  return value !== null && value !== undefined && (SUMMARY_TYPE_ORDER as string[]).includes(value)
}

/**
 * The tab to open: the one a `?lens=` link named, otherwise the first briefing
 * already readable, otherwise the reader's saved default. Called once, when
 * every input has settled — see `lensLatchReady` — because each of those three
 * arrives from a different place and answering as they land walks the open tab
 * through two wrong answers on the way to the right one.
 */
function initialLensFor(
  requested: string | null,
  lenses: SessionLens[],
  preferred: string | undefined
): SummaryType {
  if (isSummaryType(requested)) return requested
  const ready = lenses.find((lens) => lens.status === 'completed')
  if (ready) return ready.type
  return isSummaryType(preferred) ? preferred : 'general'
}

function ReaderSkeleton() {
  const { t } = useTranslation(['transcription'])
  return (
    <div className="space-y-6" role="status">
      <span className="sr-only">{t('transcription:detail.loading')}</span>
      <Skeleton className="h-8 w-64" />
      <Skeleton className="h-64 w-full" />
    </div>
  )
}

/** A session that would not load at all. There is no header to keep — the row
 *  it would describe is exactly what is missing — so this replaces the page. */
function ReaderNotFound({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation(['transcription'])
  const navigate = useNavigate()
  return (
    <div className="space-y-4">
      <Button variant="ghost" onClick={() => navigate('/library')}>
        <ArrowLeft className="mr-2 h-4 w-4" /> {t('transcription:reader.back')}
      </Button>
      <div className="py-8 text-center">
        <p className="text-destructive">{t('transcription:detail.notFound')}</p>
        <Button variant="outline" size="sm" className="mt-2" onClick={() => onRetry()}>
          {t('transcription:detail.tryAgain')}
        </Button>
      </div>
    </div>
  )
}

/** Why there is no workspace: still working, or finished badly. */
function ReaderStatusNotice({
  isInFlight,
  errorMessage,
}: {
  isInFlight: boolean
  errorMessage?: string
}) {
  const { t } = useTranslation(['transcription'])
  if (isInFlight) {
    return (
      <Card>
        <CardContent className="flex items-center justify-center gap-3 py-8">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
          <p className="text-muted-foreground">{t('transcription:detail.processing')}</p>
        </CardContent>
      </Card>
    )
  }
  if (!errorMessage) return null
  return (
    <Card>
      <CardContent className="py-6">
        <p className="text-destructive">{errorMessage}</p>
      </CardContent>
    </Card>
  )
}

/** Why the audio cannot be played, and a retry when one would help. */
function AudioStateNotice({ audio }: { audio: SessionAudio }) {
  const { t } = useTranslation(['media', 'transcription'])
  const messageKey = AUDIO_STATE_MESSAGE[audio.state]
  if (!messageKey) return null
  return (
    <div className="flex flex-wrap items-center gap-3">
      <p className="text-sm text-muted-foreground">{t(messageKey)}</p>
      {/* Only `failed` is worth retrying: a deleted file, a pending scan and a
          refused one are all answers, not transient errors. */}
      {audio.state === 'failed' && audio.retry && (
        <Button variant="outline" size="sm" onClick={audio.retry}>
          {t('transcription:detail.tryAgain')}
        </Button>
      )}
    </div>
  )
}

/** The two confirmations the header's More menu opens. Kept out of the page's
 *  return so what is left there reads as composition. */
function ReaderDialogs({
  session,
  showDelete,
  onShowDelete,
  showRetranscribe,
  onShowRetranscribe,
}: {
  session: SessionView
  showDelete: boolean
  onShowDelete: (open: boolean) => void
  showRetranscribe: boolean
  onShowRetranscribe: (open: boolean) => void
}) {
  return (
    <>
      <DeleteConfirmDialog
        open={showDelete}
        onOpenChange={onShowDelete}
        filename={session.title || session.titlePlaceholder}
        onConfirm={() => session.actions.delete?.()}
        isDeleting={session.actions.isDeleting}
      />
      {showRetranscribe && session.retranscribeSource && (
        <RetranscribeDialog
          open
          onOpenChange={onShowRetranscribe}
          transcription={session.retranscribeSource}
        />
      )}
    </>
  )
}

/**
 * The Session Reader shell: one presentational component over the `SessionView`
 * contract. It owns the page's single audio element, the briefing tab in view
 * and the dialogs — every capability, action and piece of content comes from
 * the adapter that built the view.
 */
function ReaderBody({ session }: SessionReaderPageProps) {
  const [searchParams, setSearchParams] = useSearchParams()
  const preferencesQuery = usePreferences()
  const preferences = preferencesQuery.data
  const { data: features } = useFeatures()

  const scrolledToBriefing = useRef(false)
  // The lens the URL named when this session opened. Read once, because the
  // page writes the open lens back to the URL and that write must not read as a
  // fresh deep link — it would yank the page down to the briefing band again.
  const deepLink = useRef(searchParams.get('lens'))
  const [showDelete, setShowDelete] = useState(false)
  const [showRetranscribe, setShowRetranscribe] = useState(false)
  // Null until something decides: a `?lens=` link, the first ready briefing, or
  // a reader's click. From then on it is the answer — nothing re-derives it.
  const [lensChoice, setLensChoice] = useState<SummaryType | null>(null)

  const requestedLens = deepLink.current
  const isInFlight = IN_FLIGHT_STATUSES.has(session.status)
  const isFailed = session.status === 'failed'
  const showWorkspace = !isInFlight && !isFailed
  // `available` with neither a URL nor a fetch in flight would render an empty
  // card; the adapter should not produce that, but the shell must not show it.
  const showPlayer =
    showWorkspace &&
    session.audio.state === 'available' &&
    (session.audio.streamUrl !== null || session.audio.isLoading === true)

  const playback = useReaderPlayback(showPlayer)

  /** Puts the open lens in the URL, so a refresh or a bookmark comes back to
   *  the briefing that was being read. `replace`, because stepping through tabs
   *  is not history the back button should have to walk. */
  const syncLensParam = useCallback(
    (type: SummaryType) => {
      setSearchParams(
        (current) => {
          const next = new URLSearchParams(current)
          next.set('lens', type)
          return next
        },
        { replace: true }
      )
    },
    [setSearchParams]
  )

  const chooseLens = useCallback(
    (type: SummaryType) => {
      setLensChoice(type)
      syncLensParam(type)
    },
    [syncLensParam]
  )

  const preferred = preferences?.default_summary_type
  // A `?lens=` answers on sight. Everything else waits for BOTH queries behind
  // the answer — the saved default and the briefing list — because resolving
  // against whichever landed first opens one tab, then another, then the right
  // one, under a reader who has already started reading.
  const lensLatchReady =
    isSummaryType(requestedLens) || (!preferencesQuery.isLoading && session.lensesReady !== false)

  // Latched, not derived: once anything has answered, a briefing completing
  // later cannot move the tab.
  useEffect(() => {
    if (lensChoice !== null || !lensLatchReady) return
    const resolved = initialLensFor(requestedLens, session.lenses, preferred)
    setLensChoice(resolved)
    if (requestedLens !== resolved) syncLensParam(resolved)
  }, [lensChoice, lensLatchReady, requestedLens, session.lenses, preferred, syncLensParam])

  // A `?lens=` names a briefing to read — every `/summaries/:id` link redirects
  // through one — so the band it lives in has to be on screen. Exactly one
  // nudge, once the band exists: scrolling after that belongs to the reader,
  // and re-running would yank the page back mid-read.
  useEffect(() => {
    if (scrolledToBriefing.current || !isSummaryType(requestedLens)) return
    const band = document.getElementById(BRIEFING_ANCHOR)
    if (!band) return
    scrolledToBriefing.current = true
    // `?.()` and not a call: jsdom leaves scrollIntoView undefined.
    band.scrollIntoView?.({ block: 'start' })
  }, [requestedLens, showWorkspace, session.can.briefings])

  // Until the latch fires there is nothing to be right about, so the panes get
  // the neutral lens rather than a guess that would have to move.
  const activeLens = lensChoice ?? 'general'

  // The BAP draft export needs the flag on, a media-backed transcription, and
  // a completed Q&A summary to build the draft from.
  const bapEligible =
    features?.bap_export === true &&
    session.source === 'media' &&
    session.lenses.some((lens) => lens.type === 'q_and_a' && lens.status === 'completed')

  return (
    <div className="mx-auto max-w-[1280px] space-y-8 pb-20">
      <ReaderHeader
        session={session}
        onDelete={() => setShowDelete(true)}
        onRetranscribe={() => setShowRetranscribe(true)}
      />

      {!showWorkspace && (
        <ReaderStatusNotice isInFlight={isInFlight} errorMessage={session.errorMessage} />
      )}

      <AudioStateNotice audio={session.audio} />

      {/* Outside the workspace gate on purpose: the hash and the integrity
          checks describe the uploaded file, not the transcript, and a
          transcription that failed or is still running is exactly when someone
          asks for them. The workspace renders the same panels in its rail. */}
      {!showWorkspace && (
        <div className="space-y-6">
          <ReaderFilePanels session={session} />
        </div>
      )}

      {showWorkspace && (
        <ReaderWorkspace
          session={session}
          currentTime={playback.currentTime}
          onSeek={playback.handlers.seek}
          activeLens={activeLens}
          bapEligible={bapEligible}
          columnRef={playback.transcriptRef}
          dockVisible={playback.dockVisible}
          audioCard={
            showPlayer ? (
              <AudioCard
                ref={playback.playerRef}
                cardRef={playback.cardRef}
                audio={session.audio}
                onTimeUpdate={playback.handlers.onTimeUpdate}
                onPlayStateChange={playback.handlers.onPlayStateChange}
                onDurationChange={playback.handlers.onDurationChange}
                onSpeedChange={playback.handlers.onSpeedChange}
              />
            ) : null
          }
        />
      )}

      {showWorkspace && session.can.briefings && (
        <BriefingPane
          lenses={session.lenses}
          briefing={session.briefing}
          defaultSummaryProfile={normalizeSummaryProfile(preferences?.summary_profile)}
          summaryProfilesEnabled={features?.summary_profiles === true}
          onCitationSeek={session.audio.state === 'available' ? playback.handlers.seek : undefined}
          // B contract, see plan C2: the tab is controlled from here so a
          // `?lens=` link and the export panel agree on which briefing is open.
          activeLens={activeLens}
          onActiveLensChange={chooseLens}
        />
      )}

      {playback.dockVisible && (
        <MiniDock
          isPlaying={playback.isPlaying}
          currentTime={playback.currentTime}
          duration={playback.duration}
          speed={playback.speed}
          onToggle={playback.handlers.toggle}
          onSeek={playback.handlers.seek}
          onSpeedChange={playback.handlers.setSpeed}
          onBackToTranscript={playback.handlers.backToTranscript}
        />
      )}

      <ReaderDialogs
        session={session}
        showDelete={showDelete}
        onShowDelete={setShowDelete}
        showRetranscribe={showRetranscribe}
        onShowRetranscribe={setShowRetranscribe}
      />
    </div>
  )
}

/**
 * Resolves the two states that replace the page outright, then hands the
 * session to a body keyed by its id. The key is the point: navigating from one
 * reader to another reuses this component, and the find query, the follow
 * switch, the playback speed, the open lens and the one-time briefing scroll
 * all belong to the session they were set on.
 */
export function SessionReaderPage({ session }: SessionReaderPageProps) {
  if (session.isLoading) return <ReaderSkeleton />
  if (session.isError) return <ReaderNotFound onRetry={session.refetch} />
  return <ReaderBody key={session.id} session={session} />
}
