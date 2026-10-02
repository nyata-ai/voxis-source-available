import { useState, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import type { RecordingCaptureSource, RecordingMimeType, CodecResult } from '@/types/recording'

/** Timeslice for ondataavailable (30 seconds). */
const TIMESLICE_MS = 30_000

/** Leave server-side headroom below its 25 MB recording-chunk limit. */
export const MAX_RECORDING_CHUNK_BYTES = 24 * 1024 * 1024

/** 8-hour auto-stop default limit in milliseconds. */
const DEFAULT_MAX_DURATION_MS = 8 * 60 * 60 * 1000

export function getRecordingMaxDurationMs(
  rawValue: string | undefined = import.meta.env.VITE_RECORDING_MAX_DURATION_MS,
): number {
  const parsed = Number(rawValue)
  if (Number.isFinite(parsed) && parsed > 0) {
    return Math.floor(parsed)
  }
  return DEFAULT_MAX_DURATION_MS
}

const MAX_DURATION_MS = getRecordingMaxDurationMs()

/**
 * Negotiate the best available audio codec.
 * Prefer WebM/Opus (Chrome, Firefox, Edge, Safari 18.4+), fallback MP4/AAC (older Safari).
 */
export function negotiateCodec(): CodecResult {
  if (typeof MediaRecorder === 'undefined') {
    return { supported: false, mimeType: null, codecString: null }
  }

  if (MediaRecorder.isTypeSupported('audio/webm;codecs=opus')) {
    return { supported: true, mimeType: 'audio/webm', codecString: 'audio/webm;codecs=opus' }
  }

  if (MediaRecorder.isTypeSupported('audio/mp4;codecs=mp4a.40.2')) {
    return { supported: true, mimeType: 'audio/mp4', codecString: 'audio/mp4;codecs=mp4a.40.2' }
  }

  return { supported: false, mimeType: null, codecString: null }
}

interface UseMediaRecorderOptions {
  deviceId: string
  captureSource?: RecordingCaptureSource
  /**
   * Persist one captured chunk. MUST handle its own failures: a rejection
   * here means the audio for that chunk is lost, and only the caller can
   * decide what to tell the user.
   */
  onDataAvailable: (blob: Blob, seq: number) => Promise<void> | void
  onError: (error: string) => void
  /**
   * Called when the max-duration guardrail trips. The caller is expected to
   * run its normal stop flow (which awaits `stop()`), so the final chunk is
   * written before the recorder tears down.
   */
  onAutoStop: () => void
  /** A native recorder failure has stopped capture after its final data event. */
  onCaptureInterrupted?: (error: string) => void
  onCaptureEnded?: () => void
  /** Audio became temporarily unavailable but the track has not ended. */
  onCaptureMuted?: (source: 'microphone' | 'meeting') => void
  /** Meeting capture can continue without a microphone. */
  onMicrophoneUnavailable?: () => void
}

interface ChromeDisplayMediaOptions extends DisplayMediaStreamOptions {
  systemAudio?: 'include' | 'exclude'
  windowAudio?: 'exclude' | 'window' | 'system'
  surfaceSwitching?: 'include' | 'exclude'
  selfBrowserSurface?: 'include' | 'exclude'
}

interface UseMediaRecorderReturn {
  /** Whether the browser supports recording. */
  isSupported: boolean
  /** Negotiated MIME type (null if unsupported). */
  mimeType: RecordingMimeType | null
  /** Acquire streams and prepare MediaRecorder without starting capture. */
  prepare: () => Promise<void>
  /** Start a prepared recorder. */
  start: () => Promise<void>
  /** Stop prepared local streams without completing a recording. */
  discardPrepared: () => Promise<void>
  /** Stop recording and wait for final chunk enqueue completion. */
  stop: () => Promise<void>
  /** Pause recording. */
  pause: () => void
  /** Resume recording. */
  resume: () => void
  /** Whether currently recording. */
  isRecording: boolean
  /** Whether currently paused. */
  isPaused: boolean
  /** AnalyserNode for waveform visualization. */
  analyserNode: AnalyserNode | null
  /** A prepared capture still has every required source track alive. */
  isPreparedCaptureLive: () => boolean
}

/**
 * Wraps the MediaRecorder API with codec negotiation, timeslice chunking,
 * auto-stop guardrail, and an AnalyserNode for waveform visualization.
 */
export function useMediaRecorder({
  deviceId,
  captureSource = 'microphone',
  onDataAvailable,
  onError,
  onAutoStop,
  onCaptureInterrupted,
  onCaptureEnded,
  onCaptureMuted,
  onMicrophoneUnavailable,
}: UseMediaRecorderOptions): UseMediaRecorderReturn {
  const { t } = useTranslation('recording')
  const codec = negotiateCodec()

  // Callback refs: the MediaRecorder event handlers and the auto-stop timer
  // outlive the render that created them, so they must not capture callbacks.
  const onDataAvailableRef = useRef(onDataAvailable)
  onDataAvailableRef.current = onDataAvailable
  const onErrorRef = useRef(onError)
  onErrorRef.current = onError
  const onAutoStopRef = useRef(onAutoStop)
  onAutoStopRef.current = onAutoStop
  const onCaptureInterruptedRef = useRef(onCaptureInterrupted)
  onCaptureInterruptedRef.current = onCaptureInterrupted
  const onCaptureEndedRef = useRef(onCaptureEnded)
  onCaptureEndedRef.current = onCaptureEnded
  const onCaptureMutedRef = useRef(onCaptureMuted)
  onCaptureMutedRef.current = onCaptureMuted
  const onMicrophoneUnavailableRef = useRef(onMicrophoneUnavailable)
  onMicrophoneUnavailableRef.current = onMicrophoneUnavailable
  const [isRecording, setIsRecording] = useState(false)
  const [isPaused, setIsPaused] = useState(false)
  const [analyserNode, setAnalyserNode] = useState<AnalyserNode | null>(null)

  const recorderRef = useRef<MediaRecorder | null>(null)
  const micStreamRef = useRef<MediaStream | null>(null)
  const displayStreamRef = useRef<MediaStream | null>(null)
  const recorderStreamRef = useRef<MediaStream | null>(null)
  const audioCtxRef = useRef<AudioContext | null>(null)
  const seqRef = useRef(0)
  const autoStopTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const stopResolveRef = useRef<(() => void) | null>(null)
  const dataWriteChainRef = useRef<Promise<void>>(Promise.resolve())
  const captureEndedFiredRef = useRef(false)
  const stopRequestedRef = useRef(false)
  const nativeErrorRef = useRef<string | null>(null)
  const unmountingRef = useRef(false)
  const suppressTrackCallbacksRef = useRef(false)

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      unmountingRef.current = true
      if (autoStopTimerRef.current) {
        clearTimeout(autoStopTimerRef.current)
      }
      if (recorderRef.current && recorderRef.current.state !== 'inactive') {
        stopRequestedRef.current = true
        recorderRef.current.stop()
      }
      cleanup()
    }
  }, [])

  async function prepare() {
    if (!codec.supported || !codec.codecString) {
      onError(t('recorder.unsupported'))
      throw new Error('recording unsupported')
    }

    try {
      cleanup()
      seqRef.current = 0
      captureEndedFiredRef.current = false
      stopRequestedRef.current = false
      nativeErrorRef.current = null
      unmountingRef.current = false
      suppressTrackCallbacksRef.current = false

      if (captureSource === 'mixed_audio') {
        await prepareMixedRecorder(codec.codecString)
        return
      }

      await prepareMicrophoneRecorder(codec.codecString)
    } catch (err) {
      cleanup()
      if (err instanceof DOMException && err.name === 'NotAllowedError') {
        onError(t(captureSource === 'mixed_audio' ? 'recorder.displayDenied' : 'recorder.micDenied'))
      } else if (err instanceof Error && err.message === 'display capture denied') {
        onError(t('recorder.displayDenied'))
      } else if (err instanceof Error && err.message === 'display audio unavailable') {
        onError(t('recorder.noDisplayAudio'))
      } else if (err instanceof Error && err.message === 'display capture ended') {
        onError(t('recorder.captureEndedBeforeStart'))
      } else if (err instanceof Error && err.message === 'audio context suspended') {
        onError(t('recorder.audioContextSuspended'))
      } else {
        onError(t('recorder.startFailed'))
      }
      throw err
    }
  }

  async function prepareMicrophoneRecorder(codecString: string) {
    const constraints: MediaStreamConstraints = {
      audio: deviceId ? { deviceId: { exact: deviceId } } : true,
    }
    const stream = await navigator.mediaDevices.getUserMedia(constraints)
    micStreamRef.current = stream
    watchCaptureTracks(stream, 'microphone')

    // The AudioContext only powers the cosmetic waveform on the mic path
    // (the recorder consumes the raw stream below). If the context cannot
    // reach "running" (Safari/autoplay policies), degrade the waveform
    // instead of failing the recording.
    try {
      const audioCtx = await createRunningAudioContext()
      const source = audioCtx.createMediaStreamSource(stream)
      const analyser = audioCtx.createAnalyser()
      analyser.fftSize = 256
      source.connect(analyser)
      setAnalyserNode(analyser)
    } catch {
      if (audioCtxRef.current) {
        audioCtxRef.current.close().catch(() => {})
        audioCtxRef.current = null
      }
    }

    recorderStreamRef.current = stream
    createRecorder(stream, codecString)
  }

  async function prepareMixedRecorder(codecString: string) {
    const displayOptions = {
      video: true,
      audio: true,
      systemAudio: 'include',
      windowAudio: 'system',
      surfaceSwitching: 'include',
      selfBrowserSurface: 'exclude',
    } satisfies ChromeDisplayMediaOptions

    let displayStream: MediaStream
    try {
      displayStream = await navigator.mediaDevices.getDisplayMedia(displayOptions)
    } catch (err) {
      if (err instanceof DOMException && err.name === 'NotAllowedError') {
        throw new Error('display capture denied')
      }
      throw err
    }
    displayStreamRef.current = displayStream

    const displayAudioTracks = displayStream.getAudioTracks()
    if (displayAudioTracks.length === 0) {
      for (const track of displayStream.getTracks()) {
        track.stop()
      }
      throw new Error('display audio unavailable')
    }

    let micStream: MediaStream | null = null
    if (deviceId) {
      try {
        const constraints: MediaStreamConstraints = {
          audio: { deviceId: { exact: deviceId } },
        }
        micStream = await navigator.mediaDevices.getUserMedia(constraints)
        micStreamRef.current = micStream
      } catch {
        // Display audio is still valuable for listen-only meetings. Do not turn
        // a missing or denied microphone into a failed meeting capture.
        onMicrophoneUnavailableRef.current?.()
      }
    }

    watchCaptureTracks(displayStream, 'meeting')
    if (micStream) {
      watchCaptureTracks(micStream, 'microphone')
    }

    const audioCtx = await createRunningAudioContext()
    const mixBus = audioCtx.createGain()
    const destination = audioCtx.createMediaStreamDestination()
    const analyser = audioCtx.createAnalyser()
    analyser.fftSize = 256

    const compressor = audioCtx.createDynamicsCompressor()
    compressor.threshold.value = -12
    compressor.knee.value = 20
    compressor.ratio.value = 12
    compressor.attack.value = 0.003
    compressor.release.value = 0.25

    const sourceCount = micStream ? 2 : 1
    const sourceGain = sourceCount === 2 ? 0.5 : 1
    connectWithGain(audioCtx, audioOnlyStream(displayAudioTracks), mixBus, sourceGain)
    if (micStream) {
      connectWithGain(audioCtx, micStream, mixBus, sourceGain)
    }
    mixBus.connect(compressor)
    compressor.connect(destination)
    compressor.connect(analyser)

    setAnalyserNode(analyser)
    recorderStreamRef.current = destination.stream
    createRecorder(destination.stream, codecString)
  }

  async function createRunningAudioContext(): Promise<AudioContext> {
    const audioCtx = new AudioContext()
    audioCtxRef.current = audioCtx
    if (audioCtx.state === 'suspended') {
      await audioCtx.resume()
    }
    if (audioCtx.state === 'suspended') {
      throw new Error('audio context suspended')
    }
    return audioCtx
  }

  function audioOnlyStream(tracks: MediaStreamTrack[]): MediaStream {
    if (typeof MediaStream !== 'undefined') {
      return new MediaStream(tracks)
    }
    return {
      getTracks: () => tracks,
      getAudioTracks: () => tracks,
      getVideoTracks: () => [],
    } as unknown as MediaStream
  }

  function connectWithGain(
    audioCtx: AudioContext,
    stream: MediaStream,
    destination: AudioNode,
    gainValue: number,
  ) {
    const inputGain = audioCtx.createGain()
    inputGain.gain.value = gainValue
    audioCtx.createMediaStreamSource(stream).connect(inputGain)
    inputGain.connect(destination)
  }

  function watchCaptureTracks(stream: MediaStream, source: 'microphone' | 'meeting') {
    for (const track of stream.getTracks()) {
      track.addEventListener('ended', () => {
        if (captureEndedFiredRef.current || unmountingRef.current || suppressTrackCallbacksRef.current) return
        if (source === 'meeting' && track.kind !== 'audio') {
          // Display video is only needed to make the browser sharing picker
          // available. Meeting capture stays valid while its audio track lives.
          return
        }
        if (source === 'microphone' && captureSource === 'mixed_audio') {
          // The shared meeting track remains usable without the optional mic.
          onMicrophoneUnavailableRef.current?.()
          return
        }
        captureEndedFiredRef.current = true
        onCaptureEndedRef.current?.()
      }, { once: true })
      if (track.kind !== 'audio') continue
      track.addEventListener('mute', () => {
        if (!unmountingRef.current && !suppressTrackCallbacksRef.current) {
          onCaptureMutedRef.current?.(source)
        }
      })
    }
  }

  function isPreparedCaptureLive(): boolean {
    if (!recorderRef.current || recorderRef.current.state !== 'inactive') return false
    const micTracks = micStreamRef.current?.getAudioTracks() ?? []
    if (captureSource === 'microphone') {
      return micTracks.length > 0 && micTracks.every((track) => track.readyState === 'live')
    }
    const displayTracks = displayStreamRef.current?.getAudioTracks() ?? []
    return displayTracks.length > 0 && displayTracks.every((track) => track.readyState === 'live')
  }

  function createRecorder(stream: MediaStream, codecString: string) {
    const recorder = new MediaRecorder(stream, {
      mimeType: codecString,
    })
    recorderRef.current = recorder

    recorder.ondataavailable = (event) => {
      if (event.data.size === 0) return

      // Browsers can coalesce a timeslice after sleep/backgrounding. Split the
      // original byte stream before it is admitted so every server chunk stays
      // below the upload limit without dropping any media bytes.
      for (let offset = 0; offset < event.data.size; offset += MAX_RECORDING_CHUNK_BYTES) {
        const end = Math.min(offset + MAX_RECORDING_CHUNK_BYTES, event.data.size)
        const chunk = event.data.size <= MAX_RECORDING_CHUNK_BYTES
          ? event.data
          : event.data.slice(offset, end, event.data.type)
        const seq = seqRef.current
        seqRef.current++

        // MediaRecorder callbacks do not await promises. Serialize admission to
        // IndexedDB so seq N+1 cannot become uploadable before seq N.
        dataWriteChainRef.current = dataWriteChainRef.current
          .catch(() => undefined)
          .then(async () => {
            await onDataAvailableRef.current(chunk, seq)
          })
          .catch(() => {
            // The page callback owns failure handling; keep this chain usable
            // for later pieces and avoid an unhandled rejection.
          })
      }
    }

    recorder.onerror = () => {
      // Browsers deliver error -> final dataavailable -> stop. Cleanup here
      // would race the final chunk and strand the server session.
      nativeErrorRef.current = t('recorder.errorStopped')
    }

    recorder.onstop = async () => {
      // Wait for every queued write so stop() cannot resolve while the final
      // chunk is still being persisted.
      await dataWriteChainRef.current
      const unexpectedError = nativeErrorRef.current
      const wasStopRequested = stopRequestedRef.current
      setIsRecording(false)
      setIsPaused(false)
      cleanup()
      if (stopResolveRef.current) {
        stopResolveRef.current()
        stopResolveRef.current = null
      }
      if (unexpectedError && !wasStopRequested && !unmountingRef.current) {
        onCaptureInterruptedRef.current?.(unexpectedError)
      }
    }
  }

  async function start() {
    if (!codec.supported || !codec.codecString) {
      onError(t('recorder.unsupported'))
      throw new Error('recording unsupported')
    }

    try {
      if (!recorderRef.current) {
        await prepare()
      }
      const recorder = recorderRef.current
      if (!recorder) {
        throw new Error('recorder not prepared')
      }
      if (!isPreparedCaptureLive()) {
        throw new Error('display capture ended')
      }
      recorder.start(TIMESLICE_MS)
      setIsRecording(true)
      setIsPaused(false)

      autoStopTimerRef.current = setTimeout(() => {
        if (!recorderRef.current || recorderRef.current.state === 'inactive') return
        // Do NOT stop the native recorder here: the caller's stop flow must be
        // the one that calls stop(), otherwise it sees state 'inactive',
        // resolves immediately and the final chunk write is never awaited.
        onAutoStopRef.current()
      }, MAX_DURATION_MS)
    } catch (err) {
      if (err instanceof DOMException && err.name === 'NotAllowedError') {
        onError(t('recorder.micDenied'))
      } else if (!(err instanceof Error && err.message === 'display audio unavailable')) {
        onError(t('recorder.startFailed'))
      }
      throw err
    }
  }

  async function discardPrepared() {
    cleanup()
  }

  function stop(): Promise<void> {
    if (recorderRef.current && recorderRef.current.state !== 'inactive') {
      return new Promise((resolve) => {
        stopResolveRef.current = resolve
        stopRequestedRef.current = true
        recorderRef.current?.stop()
      })
    }
    cleanup()
    return Promise.resolve()
  }

  function pause() {
    if (recorderRef.current && recorderRef.current.state === 'recording') {
      recorderRef.current.pause()
      setIsPaused(true)
    }
  }

  function resume() {
    if (recorderRef.current && recorderRef.current.state === 'paused') {
      recorderRef.current.resume()
      setIsPaused(false)
    }
  }

  function cleanup() {
    suppressTrackCallbacksRef.current = true
    if (autoStopTimerRef.current) {
      clearTimeout(autoStopTimerRef.current)
      autoStopTimerRef.current = null
    }
    const stoppedTracks = new Set<MediaStreamTrack>()
    for (const stream of [micStreamRef.current, displayStreamRef.current, recorderStreamRef.current]) {
      if (!stream) continue
      for (const track of stream.getTracks()) {
        if (stoppedTracks.has(track)) continue
        stoppedTracks.add(track)
        track.stop()
      }
    }
    micStreamRef.current = null
    displayStreamRef.current = null
    recorderStreamRef.current = null
    recorderRef.current = null
    if (audioCtxRef.current) {
      // close() rejects if the context is already closed — never let that
      // escape as an unhandled rejection.
      audioCtxRef.current.close().catch(() => {})
      audioCtxRef.current = null
    }
    setAnalyserNode(null)
    setIsRecording(false)
    setIsPaused(false)
    if (stopResolveRef.current) {
      stopResolveRef.current()
      stopResolveRef.current = null
    }
  }

  return {
    isSupported: codec.supported,
    mimeType: codec.mimeType,
    prepare,
    start,
    discardPrepared,
    stop,
    pause,
    resume,
    isRecording,
    isPaused,
    analyserNode,
    isPreparedCaptureLive,
  }
}
