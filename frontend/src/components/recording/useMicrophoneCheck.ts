import { useCallback, useEffect, useRef, useState } from 'react'

/** The check listens for at most this long, then lets go of the microphone. */
export const MIC_CHECK_MS = 10_000
/** Silence is reported only after this long, so the first breath is not a fail. */
const SILENCE_GRACE_MS = 3_000
/**
 * A sample at or above this counts as hearing the user. On the tape's
 * -60…-10 dBFS scale, 0.4 is -40 dBFS: above a quiet room (about -60 to -50)
 * and below speech at a normal distance (about -30 to -15), now that the
 * samples are float and a quiet input really reads near zero.
 */
const HEARD_LEVEL = 0.4

export type CheckState = 'idle' | 'starting' | 'listening' | 'done'

interface Capture {
  stream: MediaStream
  context: AudioContext
  analyser: AnalyserNode
}

function stopStream(stream: MediaStream | null): void {
  if (!stream) return
  for (const track of stream.getTracks()) track.stop()
}

function closeContext(context: AudioContext | null): void {
  if (context) void context.close().catch(() => {})
}

/**
 * Opens the microphone and an analyser on it. Null if nobody wants it any more
 * by the time the browser answers; everything it opened is let go then, and when
 * it throws.
 */
async function openCapture(deviceId: string, isWanted: () => boolean): Promise<Capture | null> {
  let stream: MediaStream | null = null
  let context: AudioContext | null = null
  const abandon = () => {
    stopStream(stream)
    closeContext(context)
  }
  try {
    stream = await navigator.mediaDevices.getUserMedia({
      audio: deviceId ? { deviceId: { exact: deviceId } } : true,
    })
    if (!isWanted()) {
      abandon()
      return null
    }
    context = new AudioContext()
    if (context.state === 'suspended') await context.resume()
    if (!isWanted()) {
      abandon()
      return null
    }
    const analyser = context.createAnalyser()
    analyser.fftSize = 256
    context.createMediaStreamSource(stream).connect(analyser)
    return { stream, context, analyser }
  } catch (error) {
    abandon()
    throw error
  }
}

/** The microphone check's capture. See `MicrophoneCheck` for when it lets go. */
export function useMicrophoneCheck(deviceId: string, disabled: boolean) {
  const [state, setState] = useState<CheckState>('idle')
  const [heard, setHeard] = useState(false)
  const [silent, setSilent] = useState(false)
  const [failed, setFailed] = useState(false)
  const [analyser, setAnalyser] = useState<AnalyserNode | null>(null)
  const captureRef = useRef<Capture | null>(null)
  const timersRef = useRef<number[]>([])
  // Each start takes a token; `release` moves it on, so a start still waiting
  // on the browser can tell, when it resumes, that nobody wants its stream.
  const runRef = useRef(0)

  const release = useCallback(() => {
    runRef.current += 1
    for (const timer of timersRef.current) window.clearTimeout(timer)
    timersRef.current = []
    const capture = captureRef.current
    captureRef.current = null
    if (!capture) return
    stopStream(capture.stream)
    closeContext(capture.context)
  }, [])

  const reset = useCallback(() => {
    release()
    setAnalyser(null)
    setHeard(false)
    setSilent(false)
    setFailed(false)
    setState('idle')
  }, [release])

  useEffect(() => release, [release])

  // A result for the old microphone says nothing about the new one, and a
  // check in progress must not keep listening to a device the user left.
  const lastDeviceRef = useRef(deviceId)
  useEffect(() => {
    if (lastDeviceRef.current === deviceId) return
    lastDeviceRef.current = deviceId
    reset()
  }, [deviceId, reset])

  useEffect(() => {
    if (disabled) reset()
  }, [disabled, reset])

  const finish = useCallback(() => {
    release()
    setAnalyser(null)
    setState('done')
  }, [release])

  async function start() {
    release()
    const run = runRef.current
    setHeard(false)
    setSilent(false)
    setFailed(false)
    setState('starting')
    try {
      const capture = await openCapture(deviceId, () => run === runRef.current)
      if (!capture) return
      captureRef.current = capture
      setAnalyser(capture.analyser)
      setState('listening')
      timersRef.current = [
        window.setTimeout(() => setSilent(true), SILENCE_GRACE_MS),
        window.setTimeout(finish, MIC_CHECK_MS),
      ]
    } catch {
      // A newer start or the unmount already owns the state.
      if (run !== runRef.current) return
      release()
      setFailed(true)
      setState('done')
    }
  }

  const onLevel = useCallback((level: number) => {
    if (level >= HEARD_LEVEL) setHeard(true)
  }, [])

  return { state, heard, silent, failed, analyser, start, onLevel }
}
