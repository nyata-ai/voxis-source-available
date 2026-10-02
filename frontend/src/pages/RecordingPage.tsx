import { useState, useEffect, useRef } from 'react'
import { Link, useBlocker, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Mic, Pause, Play, Square, Loader2, AlertTriangle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { StorageQuotaNotice } from '@/components/media/StorageQuotaNotice'
import { BrowserCompatCheck } from '@/components/recording/BrowserCompatCheck'
import { CaptureSourceSelector } from '@/components/recording/CaptureSourceSelector'
import { MicrophoneSelector } from '@/components/recording/MicrophoneSelector'
import { RecordingTimer } from '@/components/recording/RecordingTimer'
import { RecordingWaveform } from '@/components/recording/RecordingWaveform'
import { RecoverRecordingDialog } from '@/components/recording/RecoverRecordingDialog'
import { RecordingExitDialog } from '@/components/recording/RecordingExitDialog'
import { ActiveSessionTakeoverDialog } from '@/components/recording/ActiveSessionTakeoverDialog'
import { MeetingCaptureConsentDialog } from '@/components/recording/MeetingCaptureConsentDialog'
import {
  RecordingDataLossDialog,
  type RecordingDataLossAction,
} from '@/components/recording/RecordingDataLossDialog'
import { useMicrophoneDevices } from '@/hooks/useMicrophoneDevices'
import { useMediaRecorder } from '@/hooks/useMediaRecorder'
import { useChunkUpload } from '@/hooks/useChunkUpload'
import { useInterruptedRecording } from '@/hooks/useInterruptedRecording'
import { useUsageStats } from '@/hooks/useUsageStats'
import {
  useCreateRecordingSession,
  useCompleteRecordingSession,
  usePauseRecordingSession,
  useResumeRecordingSession,
  useRecoverRecordingSession,
  useReleaseRecordingSession,
  useAbandonRecordingSession,
  fetchActiveRecordingSession,
  sendHeartbeat,
} from '@/hooks/useRecordingSession'
import { useRecordingStore, type RecordingFlushFailureCode } from '@/stores/recording'
import {
  cleanupOldEntries,
  getAllBySession,
  deleteBySession,
  deleteChunk,
} from '@/lib/chunk-outbox'
import { getActionableApiErrorMessage, httpStatusOf, isPermanentHttpError } from '@/lib/api-errors'
import { isHeartbeatCadenceSafe, shouldBlockRecordingExit } from '@/lib/recording-exit-guard'
import { toast } from '@/lib/toast'
import { uploadChunk } from '@/hooks/useRecordingSession'
import type { RecordingSession } from '@/types/recording'

const TITLE_UPDATE_INTERVAL_MS = 10_000
const DEFAULT_HEARTBEAT_INTERVAL_MS = 45_000
// Sub-second captures stitch to files below the backend's 1024-byte media
// minimum and can never be saved; gate Stop until the recording is viable.
export const MIN_RECORDING_SECONDS = 2
const DEFAULT_ORPHAN_THRESHOLD_MINUTES = 30

function parseDurationEnv(raw: string | undefined, fallbackMs: number): number {
  const parsed = Number(raw)
  if (Number.isFinite(parsed) && parsed > 0) {
    return Math.floor(parsed)
  }
  return fallbackMs
}

const HEARTBEAT_INTERVAL_MS = parseDurationEnv(
  import.meta.env.VITE_RECORDING_HEARTBEAT_INTERVAL_MS,
  DEFAULT_HEARTBEAT_INTERVAL_MS
)

export function RecordingPage() {
  const { t } = useTranslation(['recording', 'errors'])
  const navigate = useNavigate()
  const store = useRecordingStore()
  const { data: usageStats, refetch: refetchUsage } = useUsageStats()

  // Cleanup old IndexedDB entries on mount
  useEffect(() => {
    cleanupOldEntries().catch(() => {})
  }, [])

  // Interrupted sessions
  const { data: interruptedData, refetch: refetchInterrupted } = useInterruptedRecording()
  const interruptedSessions = interruptedData?.items ?? []
  const [showRecovery, setShowRecovery] = useState(false)
  const [isExiting, setIsExiting] = useState(false)
  const [isRecovering, setIsRecovering] = useState(false)
  const [dataLossAction, setDataLossAction] = useState<RecordingDataLossAction | null>(null)
  const [showMeetingConsent, setShowMeetingConsent] = useState(false)
  const [meetingConsentAcknowledged, setMeetingConsentAcknowledged] = useState(false)

  useEffect(() => {
    if (interruptedSessions.length > 0 && store.status === 'idle') {
      setShowRecovery(true)
    }
  }, [interruptedSessions.length, store.status])

  // Session mutations
  const createSession = useCreateRecordingSession()
  const completeSession = useCompleteRecordingSession()
  const pauseSession = usePauseRecordingSession()
  const resumeSession = useResumeRecordingSession()
  const recoverSession = useRecoverRecordingSession()
  const releaseSession = useReleaseRecordingSession()
  const abandonSession = useAbandonRecordingSession()

  // Microphone devices
  const {
    devices,
    selectedDeviceId,
    setSelectedDeviceId,
    error: micError,
    isLoading: micLoading,
  } = useMicrophoneDevices()

  // MediaRecorder
  const recorder = useMediaRecorder({
    deviceId: selectedDeviceId,
    captureSource: store.captureSource,
    onDataAvailable: async (blob, seq) => {
      try {
        await chunkUpload.enqueue(seq, blob)
      } catch {
        handleChunkWriteFailure()
      }
    },
    onError: (error) => {
      store.setError(error)
      toast.error(error)
    },
    onAutoStop: () => {
      void handleStop()
      toast.info(t('toast.autoStop'))
    },
    onCaptureEnded: () => {
      void handleStop()
      toast.info(t('toast.captureEnded'))
    },
    onCaptureInterrupted: (error) => {
      void handleCaptureInterrupted(error)
    },
    onCaptureMuted: (source) => {
      toast.info(t(source === 'meeting' ? 'toast.meetingAudioMuted' : 'toast.microphoneMuted'))
    },
    onMicrophoneUnavailable: () => {
      toast.info(t('toast.meetingWithoutMicrophone'))
    },
  })

  // Chunk upload
  const chunkUpload = useChunkUpload({
    sessionId: store.sessionId,
    onBacklogExceeded: () => {
      if (store.status === 'recording') {
        recorder.pause()
        if (store.sessionId) {
          pauseSession.mutate(store.sessionId)
        }
        store.setStatus('paused')
        toast.error(t('toast.uploadInterrupted'))
      }
    },
  })

  // Elapsed timer (runs every second when recording, pauses when paused)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    if (store.status === 'recording') {
      timerRef.current = setInterval(() => {
        store.incrementElapsed()
      }, 1000)
    } else {
      if (timerRef.current) {
        clearInterval(timerRef.current)
        timerRef.current = null
      }
    }
    return () => {
      if (timerRef.current) {
        clearInterval(timerRef.current)
      }
    }
  }, [store.status]) // eslint-disable-line react-hooks/exhaustive-deps

  // Update document title every 10 seconds during recording
  useEffect(() => {
    if (store.status !== 'recording' && store.status !== 'paused') {
      document.title = t('layout.brand')
      return
    }

    const updateTitle = () => {
      const prefix =
        store.status === 'paused' ? t('documentTitle.paused') : t('documentTitle.recording')
      document.title = t('documentTitle.withApp', { prefix })
    }
    updateTitle()
    const interval = setInterval(updateTitle, TITLE_UPDATE_INTERVAL_MS)
    return () => {
      clearInterval(interval)
      document.title = t('layout.brand')
    }
  }, [store.status, t])

  // Heartbeat while the server session is alive. That includes 'completing':
  // the session stays `recording` server-side until the complete call returns,
  // and chunk uploads only bump last_chunk_at — so a long drain without
  // heartbeats gets orphan-swept mid-completion.
  useEffect(() => {
    const sessionId = store.sessionId
    if (!sessionId) return
    if (
      store.status !== 'recording' &&
      store.status !== 'paused' &&
      store.status !== 'completing'
    ) {
      return
    }
    if (!isHeartbeatCadenceSafe(HEARTBEAT_INTERVAL_MS, DEFAULT_ORPHAN_THRESHOLD_MINUTES)) return

    const interval = setInterval(() => {
      void sendHeartbeat(sessionId).catch(() => {})
    }, HEARTBEAT_INTERVAL_MS)

    return () => clearInterval(interval)
  }, [store.sessionId, store.status])

  // beforeunload warning during recording
  useEffect(() => {
    if (store.status === 'idle' || store.status === 'error') return

    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault()
    }

    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [store.status])

  // In-app navigation guard. beforeunload does not cover SPA navigation, so the
  // header's Cancel/logo links would otherwise unmount the recorder mid-capture.
  const blocker = useBlocker(shouldBlockRecordingExit)
  const isExitBlocked = blocker.state === 'blocked'

  // A session the server still reports as active locks the user out of
  // recording until the orphan sweep runs. It may equally be a healthy
  // recording in another tab, so the session is surfaced for an explicit
  // decision instead of being interrupted on sight.
  const [activeSession, setActiveSession] = useState<RecordingSession | null>(null)
  const [isTakingOver, setIsTakingOver] = useState(false)
  const activeSessionCheckedRef = useRef(false)
  useEffect(() => {
    if (activeSessionCheckedRef.current) return
    activeSessionCheckedRef.current = true
    void detectActiveSession()
  }, [])

  // Outbox drain progress: total captured at stop time; the remaining count
  // comes live from the store so the counter actually moves.
  const [drainTotal, setDrainTotal] = useState<number | null>(null)

  // Re-entrancy guard for handleStop. A ref (not state) because the native
  // "Stop sharing" control can invoke stop multiple times in the same tick.
  const stopInFlightRef = useRef(false)
  const captureInterruptionInFlightRef = useRef(false)

  /**
   * Ask the server whether a session is still active for this user. A fresh
   * mount only proves *this* tab does not own it — another tab may still be
   * recording — so the answer is surfaced, never acted on.
   */
  async function detectActiveSession() {
    if (useRecordingStore.getState().status !== 'idle') return
    try {
      const active = await fetchActiveRecordingSession()
      if (!active) return
      setActiveSession(active)
    } catch {
      // Fail soft: behave as if there is none. The server's orphan sweeper is
      // the backstop, and Start still surfaces the 409 if one really exists.
    }
  }

  /** User accepted interrupting the session; hand it to the recovery flow. */
  async function handleTakeOverActiveSession() {
    const session = activeSession
    if (!session) return
    setIsTakingOver(true)
    try {
      await releaseSession.mutateAsync(session.id)
      setActiveSession(null)
      toast.info(t('toast.releasedActiveSession'))
    } catch {
      toast.error(t('toast.takeOverFailed'))
    } finally {
      setIsTakingOver(false)
    }
  }

  /** User declined: leave the other tab's session alone and go elsewhere. */
  function handleLeaveActiveSession() {
    setActiveSession(null)
    navigate('/library')
  }

  function handleChunkWriteFailure() {
    const live = useRecordingStore.getState()
    if (live.localWriteFailed) return // one warning is enough
    live.setLocalWriteFailed(true)
    toast.error(t('toast.chunkWriteFailed'))
    // Stopping keeps everything captured so far; continuing guarantees a gap
    // that also wedges the server's strict chunk sequencing.
    void handleStop()
  }

  /** Preserve buffered audio after a browser-level recorder failure. */
  async function handleCaptureInterrupted(error: string) {
    if (captureInterruptionInFlightRef.current) return
    const liveStatus = useRecordingStore.getState().status
    if (liveStatus !== 'recording' && liveStatus !== 'paused') return
    captureInterruptionInFlightRef.current = true

    const sessionId = useRecordingStore.getState().sessionId
    try {
      store.setStatus('completing')
      await recorder.stop()
      if (!sessionId) return
      try {
        await releaseSession.mutateAsync(sessionId)
      } catch (releaseError) {
        // A 409 means the orphan sweep already moved it to interrupted; the
        // recovery query below is still the safe source of truth.
        if (httpStatusOf(releaseError) !== 409) throw releaseError
      }
      await refetchInterrupted()
      store.reset()
      setShowRecovery(true)
      toast.error(t('toast.captureInterrupted', { error }))
    } catch {
      // Keep session/outbox references intact. A retry after connectivity is
      // preferable to silently completing a session whose final chunk is
      // unknown.
      store.setError(t('toast.captureRecoveryFailed'))
      toast.error(t('toast.captureRecoveryFailed'))
    } finally {
      captureInterruptionInFlightRef.current = false
    }
  }

  async function handleStart(consentAlreadyAcknowledged = false) {
    if (!recorder.mimeType) return
    if (
      store.captureSource === 'mixed_audio' &&
      !meetingConsentAcknowledged &&
      !consentAlreadyAcknowledged
    ) {
      setShowMeetingConsent(true)
      return
    }

    const latestUsage = (await refetchUsage()).data ?? usageStats
    if (latestUsage?.user_storage?.state === 'full') return

    try {
      await recorder.prepare()
      if (!recorder.isPreparedCaptureLive()) {
        await recorder.discardPrepared()
        toast.error(t('recorder.captureEndedBeforeStart'))
        return
      }
    } catch {
      return
    }

    let sessionId: string | null = null
    try {
      const session = await createSession.mutateAsync({
        mime_type: recorder.mimeType,
        microphone_label: devices.find((d) => d.deviceId === selectedDeviceId)?.label,
        capture_source: store.captureSource,
      })
      sessionId = session.id
      store.setSessionId(session.id)
      store.setMimeType(recorder.mimeType)
      store.setElapsed(0)
    } catch (err) {
      await recorder.discardPrepared()
      // 409 means another session is still active — a generic "failed to
      // start" hides the one thing the user can act on.
      const message =
        httpStatusOf(err) === 409
          ? t('toast.activeSessionExists')
          : (getActionableApiErrorMessage(err, t) ?? t('toast.startSessionFailed'))
      store.setError(message)
      toast.error(message)
      return
    }

    try {
      if (!recorder.isPreparedCaptureLive()) {
        throw new Error('capture source ended')
      }
      await recorder.start()
      store.setStatus('recording')
    } catch {
      // No audio has started yet. Release then abandon rather than attempting to
      // stitch an empty session or leaving it as the caller's active session.
      if (sessionId) {
        try {
          await releaseSession.mutateAsync(sessionId)
          await abandonSession.mutateAsync(sessionId)
        } catch {
          // The server's recovery sweep remains the fallback if release failed.
        }
      }
      store.reset()
      store.setError(t('toast.startFailed'))
      toast.error(t('toast.startFailed'))
    }
  }

  async function handleStop() {
    // Ignore re-entry: a second concurrent run would complete the session
    // again and surface a spurious 409 error after the first run succeeded.
    if (stopInFlightRef.current) return
    // Read live store state, not the render snapshot: capture-ended and
    // auto-stop callbacks run from closures captured at prepare() time,
    // when the store still said idle with no session.
    const liveStatus = useRecordingStore.getState().status
    if (liveStatus !== 'recording' && liveStatus !== 'paused') return
    stopInFlightRef.current = true

    try {
      store.setStatus('completing')
      await recorder.stop()
      warnIfLocalWriteFailed()

      // Wait for outbox to drain
      setDrainTotal(useRecordingStore.getState().outboxSize)
      try {
        await chunkUpload.waitForDrain()
      } catch {
        // Permanently rejected chunk. Hold on the finalizing screen (the store's
        // flushFailure drives the Retry / give-up UI) instead of hanging.
        setDrainTotal(null)
        return
      }
      setDrainTotal(null)

      await finalizeSession()
    } finally {
      stopInFlightRef.current = false
    }
  }

  /** Tell the user when locally dropped audio means a truncated tail. */
  function warnIfLocalWriteFailed() {
    if (!useRecordingStore.getState().localWriteFailed) return
    toast.error(t('toast.trailingAudioMissing'))
  }

  async function finalizeSession() {
    const sessionId = useRecordingStore.getState().sessionId
    if (!sessionId) return
    try {
      await completeSession.mutateAsync(sessionId)
      await chunkUpload.clearSession().catch(() => {})
      store.reset()
      setMeetingConsentAcknowledged(false)
      navigate('/library')
    } catch (err) {
      // During an extended outage the server can orphan the session while this
      // page keeps recording. It accepts the flushed chunks, but Complete only
      // accepts active sessions; hand that exact state to Recover instead.
      if (httpStatusOf(err) === 400 || httpStatusOf(err) === 409) {
        try {
          await recoverSession.mutateAsync(sessionId)
          await deleteBySession(sessionId).catch(() => {})
          store.reset()
          setMeetingConsentAcknowledged(false)
          toast.info(t('toast.recoveredAfterOffline'))
          navigate('/library')
          return
        } catch {
          // Keep the original completion error below when this was not an
          // interrupted session or recovery could not be scheduled.
        }
      }
      store.setError(t('toast.completeFailedStore'))
      toast.error(getActionableApiErrorMessage(err, t) ?? t('toast.completeFailed'))
    }
  }

  async function handleRetryUpload() {
    setDrainTotal(useRecordingStore.getState().outboxSize)
    try {
      await chunkUpload.retryFlush()
      setDrainTotal(null)
    } catch {
      // Still permanently rejected — the failure panel stays up.
      setDrainTotal(null)
      return
    }
    await finalizeSession()
  }

  /** Give up on the undeliverable tail and keep what the server already has. */
  async function handleSaveUploaded() {
    setDataLossAction(null)
    setIsExiting(true)
    try {
      await chunkUpload.clearSession().catch(() => {})
      await finalizeSession()
    } finally {
      setIsExiting(false)
    }
  }

  /**
   * Throw the recording away: release the still-active server session so it
   * becomes interruptible, abandon it, then drop the local chunks.
   */
  async function handleDiscardActive() {
    setDataLossAction(null)
    setIsExiting(true)
    const sessionId = useRecordingStore.getState().sessionId
    try {
      await recorder.stop()
      if (sessionId) {
        try {
          await releaseSession.mutateAsync(sessionId)
        } catch {
          // Already interrupted or swept — abandon still applies.
        }
        try {
          await abandonSession.mutateAsync(sessionId)
        } catch {
          toast.error(t('toast.discardFailed'))
        }
      }
      await chunkUpload.clearSession().catch(() => {})
      store.reset()
      navigate('/library')
    } finally {
      setIsExiting(false)
    }
  }

  function handlePause() {
    recorder.pause()
    if (store.sessionId) {
      pauseSession.mutate(store.sessionId)
    }
    store.setStatus('paused')
  }

  function handleResume() {
    recorder.resume()
    if (store.sessionId) {
      resumeSession.mutate(store.sessionId)
    }
    store.setStatus('recording')
  }

  /** Keep recording: dismiss the guard and stay on the page. */
  function handleKeepRecording() {
    blocker.reset?.()
  }

  /**
   * Stop & save from the exit guard. The blocked navigation is reset rather
   * than proceeded: handleStop navigates to the library itself once the
   * session is complete, and both guarded links point there anyway.
   */
  function handleExitStopAndSave() {
    blocker.reset?.()
    void handleStop()
  }

  function handleExitDiscard() {
    blocker.reset?.()
    setDataLossAction('discard')
  }

  async function handleRecover(sessionId: string) {
    setIsRecovering(true)
    try {
      // Upload locally buffered chunks first to recover audio captured before
      // crash/reload. Duplicate seq retries are idempotent (200) server-side.
      const localEntries = await getAllBySession(sessionId)
      localEntries.sort((a, b) => a.seq - b.seq)

      let unattached = 0
      for (const entry of localEntries) {
        try {
          await uploadChunk(sessionId, entry.seq, entry.blob)
        } catch (err) {
          // A permanently rejected chunk (e.g. 409 because a middle chunk was
          // lost locally) must not abort the whole recovery: the server-side
          // audio is still worth saving. Keep the local copy for diagnosis.
          if (isPermanentHttpError(err)) {
            unattached++
            continue
          }
          throw err
        }
        if (entry.id !== undefined) {
          await deleteChunk(entry.id)
        }
      }

      if (unattached > 0) {
        toast.error(t('toast.recoverPartialUpload', { n: unattached }))
      }

      await recoverSession.mutateAsync(sessionId)
      await deleteBySession(sessionId).catch(() => {})
      setShowRecovery(false)
      toast.success(t('toast.recovered'))
      navigate('/library')
    } catch {
      toast.error(t('toast.recoverFailed'))
    } finally {
      setIsRecovering(false)
    }
  }

  async function handleDiscard(sessionId: string) {
    try {
      await abandonSession.mutateAsync(sessionId)
      await deleteBySession(sessionId)
      const remaining = interruptedSessions.filter((s) => s.id !== sessionId)
      if (remaining.length === 0) {
        setShowRecovery(false)
      }
      toast.success(t('toast.discarded'))
    } catch {
      toast.error(t('toast.discardFailed'))
    }
  }

  return (
    <BrowserCompatCheck>
      <div className="flex flex-col items-center justify-center min-h-[calc(100vh-4rem)] px-4">
        {/* Someone (maybe another tab) still owns an active session */}
        <ActiveSessionTakeoverDialog
          session={activeSession}
          onTakeOver={() => void handleTakeOverActiveSession()}
          onLeave={handleLeaveActiveSession}
          isBusy={isTakingOver || releaseSession.isPending}
        />

        {/* Recovery dialog */}
        <RecoverRecordingDialog
          open={showRecovery && !activeSession}
          onOpenChange={setShowRecovery}
          sessions={interruptedSessions}
          onRecover={handleRecover}
          onDiscard={handleDiscard}
          isRecovering={isRecovering || recoverSession.isPending}
          isDiscarding={abandonSession.isPending}
        />

        {/* In-app navigation guard */}
        <RecordingExitDialog
          open={isExitBlocked}
          onKeepRecording={handleKeepRecording}
          onStopAndSave={handleExitStopAndSave}
          onDiscard={handleExitDiscard}
          isBusy={isExiting}
        />

        {/* Irreversible give-up confirmations */}
        <RecordingDataLossDialog
          action={dataLossAction}
          onOpenChange={(open) => {
            if (!open) setDataLossAction(null)
          }}
          onConfirm={() => {
            if (dataLossAction === 'discard') {
              void handleDiscardActive()
            } else {
              void handleSaveUploaded()
            }
          }}
          isBusy={isExiting}
        />

        <MeetingCaptureConsentDialog
          open={showMeetingConsent}
          onCancel={() => setShowMeetingConsent(false)}
          onConfirm={() => {
            setMeetingConsentAcknowledged(true)
            setShowMeetingConsent(false)
            void handleStart(true)
          }}
        />

        {/* State 1: Pre-recording / Idle */}
        {store.status === 'idle' && (
          <div className="flex flex-col items-center gap-6 text-center">
            <div className="flex h-24 w-24 items-center justify-center rounded-full bg-primary/10">
              <Mic className="h-12 w-12 text-primary" />
            </div>
            <div className="space-y-2">
              <h1 className="text-2xl font-normal">{t('idle.title')}</h1>
              {micLoading ? (
                <p className="text-muted-foreground">{t('idle.detecting')}</p>
              ) : (
                <div className="flex flex-col items-center gap-4">
                  {micError && <p className="text-destructive text-sm">{micError}</p>}
                  {!micError && (
                    <MicrophoneSelector
                      devices={devices}
                      selectedDeviceId={selectedDeviceId}
                      onDeviceChange={setSelectedDeviceId}
                    />
                  )}
                  <CaptureSourceSelector
                    value={store.captureSource}
                    onChange={store.setCaptureSource}
                  />
                </div>
              )}
            </div>
            <StorageQuotaNotice storage={usageStats?.user_storage} className="max-w-xl text-left" />
            <Button
              size="lg"
              onClick={() => void handleStart()}
              disabled={
                micLoading ||
                (store.captureSource === 'microphone' && (!!micError || devices.length === 0)) ||
                createSession.isPending ||
                // A pending take-over decision must not race a new session.
                activeSession !== null ||
                isTakingOver
              }
              className="gap-2 px-8"
            >
              {createSession.isPending ? (
                <Loader2 className="h-5 w-5 animate-spin" />
              ) : (
                <Mic className="h-5 w-5" />
              )}
              {t('idle.start')}
            </Button>
          </div>
        )}

        {/* State 2: Recording active (or paused) */}
        {(store.status === 'recording' || store.status === 'paused') && (
          <div className="flex flex-col items-center gap-8">
            {/* Status indicator */}
            <div className="flex items-center gap-2">
              {store.status === 'recording' ? (
                <>
                  <span className="h-3 w-3 rounded-full bg-live animate-pulse" />
                  <span className="text-sm font-medium text-live">{t('status.recording')}</span>
                </>
              ) : (
                <>
                  <span className="h-3 w-3 rounded-full bg-warning" />
                  <span className="text-sm font-medium text-warning">{t('status.paused')}</span>
                </>
              )}
            </div>

            {/* Timer */}
            <RecordingTimer elapsed={store.elapsed} isPaused={store.status === 'paused'} />

            {/* Waveform */}
            <RecordingWaveform
              analyserNode={recorder.analyserNode}
              isPaused={store.status === 'paused'}
            />

            {/* Controls */}
            <div className="flex items-center gap-4">
              {store.status === 'recording' ? (
                <Button variant="outline" size="lg" onClick={handlePause} className="gap-2">
                  <Pause className="h-5 w-5" />
                  {t('controls.pause')}
                </Button>
              ) : (
                <Button variant="outline" size="lg" onClick={handleResume} className="gap-2">
                  <Play className="h-5 w-5" />
                  {t('controls.resume')}
                </Button>
              )}
              <Button
                variant="destructive"
                size="lg"
                onClick={handleStop}
                disabled={store.elapsed < MIN_RECORDING_SECONDS}
                className="gap-2"
              >
                <Square className="h-5 w-5" />
                {t('controls.stop')}
              </Button>
            </div>

            {store.elapsed < MIN_RECORDING_SECONDS && (
              <p className="text-xs text-muted-foreground">
                {t('controls.minDurationHint', { seconds: MIN_RECORDING_SECONDS })}
              </p>
            )}

            {/* Upload status */}
            <UploadStatusIndicator outboxSize={store.outboxSize} lastSavedAt={store.lastSavedAt} />
          </div>
        )}

        {/* State 3: Processing (after Stop) */}
        {store.status === 'completing' && (
          <div className="flex flex-col items-center gap-6 text-center">
            {store.flushFailure ? (
              <FlushFailurePanel
                status={store.flushFailure.status}
                code={store.flushFailure.code}
                remaining={store.outboxSize}
                isBusy={isExiting}
                onRetry={() => void handleRetryUpload()}
                onSaveUploaded={() => setDataLossAction('savePartial')}
                onDiscard={() => setDataLossAction('discard')}
              />
            ) : (
              <>
                <Loader2 className="h-12 w-12 animate-spin text-primary" />
                <div className="space-y-2">
                  <h2 className="text-xl font-semibold">{t('completing.title')}</h2>
                  {drainTotal !== null && store.outboxSize > 0 ? (
                    <p className="text-muted-foreground">
                      {t('completing.uploadingRemaining', {
                        done: Math.max(0, drainTotal - store.outboxSize),
                        total: drainTotal,
                      })}
                    </p>
                  ) : (
                    <p className="text-muted-foreground">{t('completing.processing')}</p>
                  )}
                  {store.localWriteFailed && (
                    <p className="text-sm text-destructive">{t('completing.localWriteWarning')}</p>
                  )}
                </div>
              </>
            )}
          </div>
        )}

        {/* Error state */}
        {store.status === 'error' && (
          <div className="flex flex-col items-center gap-6 text-center">
            <div className="space-y-2">
              <h2 className="text-xl font-semibold text-destructive">{t('error.title')}</h2>
              <p className="text-muted-foreground">{store.errorMessage || t('error.fallback')}</p>
            </div>
            <Button
              variant="outline"
              onClick={() => {
                store.reset()
                activeSessionCheckedRef.current = false
                void detectActiveSession()
              }}
            >
              {t('error.tryAgain')}
            </Button>
          </div>
        )}
      </div>
    </BrowserCompatCheck>
  )
}

/**
 * Terminal upload failure on the finalizing screen: the server will keep
 * rejecting the queued audio, so the user needs a way forward, not a spinner.
 */
function FlushFailurePanel({
  status,
  code,
  remaining,
  isBusy,
  onRetry,
  onSaveUploaded,
  onDiscard,
}: {
  status: number
  code?: RecordingFlushFailureCode
  remaining: number
  isBusy: boolean
  onRetry: () => void
  onSaveUploaded: () => void
  onDiscard: () => void
}) {
  const { t } = useTranslation(['recording', 'media', 'errors'])
  const storageQuotaExceeded = code === 'storage_quota_exceeded'

  return (
    <>
      <AlertTriangle className="h-12 w-12 text-destructive" />
      <div className="space-y-2">
        <h2 className="text-xl font-semibold">
          {storageQuotaExceeded ? t('media:storageQuota.full') : t('completing.uploadFailedTitle')}
        </h2>
        {!storageQuotaExceeded && (
          <p className="text-muted-foreground max-w-md">
            {code === 'media_too_long'
              ? t('errors:codes.media_too_long')
              : t('completing.uploadFailedBody', { status, n: remaining })}
          </p>
        )}
      </div>
      <div className="flex flex-wrap items-center justify-center gap-3">
        <Button onClick={onRetry} disabled={isBusy}>
          {t('completing.retryUpload')}
        </Button>
        {storageQuotaExceeded && (
          <Link
            to="/library"
            target="_blank"
            rel="noopener noreferrer"
            className="text-sm font-medium underline underline-offset-2"
          >
            {t('media:storageQuota.manageLibrary')}
          </Link>
        )}
        <Button variant="outline" onClick={onSaveUploaded} disabled={isBusy}>
          {t('completing.saveUploaded')}
        </Button>
        <Button variant="destructive" onClick={onDiscard} disabled={isBusy}>
          {t('completing.discardRecording')}
        </Button>
      </div>
    </>
  )
}

function UploadStatusIndicator({
  outboxSize,
  lastSavedAt,
}: {
  outboxSize: number
  lastSavedAt: number | null
}) {
  const { t } = useTranslation('recording')
  if (outboxSize > 0) {
    return <p className="text-sm text-warning">{t('upload.saving', { n: outboxSize })}</p>
  }

  if (lastSavedAt) {
    return <p className="text-sm text-muted-foreground">{t('upload.saved')}</p>
  }

  return null
}
