import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import {
  negotiateCodec,
  useMediaRecorder,
  getRecordingMaxDurationMs,
  MAX_RECORDING_CHUNK_BYTES,
} from './useMediaRecorder'

describe('negotiateCodec', () => {
  const originalMediaRecorder = globalThis.MediaRecorder

  afterEach(() => {
    if (originalMediaRecorder) {
      vi.stubGlobal('MediaRecorder', originalMediaRecorder)
    } else {
      vi.unstubAllGlobals()
    }
  })

  it('returns supported=false when MediaRecorder is undefined', () => {
    vi.stubGlobal('MediaRecorder', undefined)
    const result = negotiateCodec()
    expect(result.supported).toBe(false)
    expect(result.mimeType).toBeNull()
    expect(result.codecString).toBeNull()
  })

  it('prefers WebM/Opus when supported', () => {
    vi.stubGlobal('MediaRecorder', {
      isTypeSupported: (type: string) => type === 'audio/webm;codecs=opus',
    })
    const result = negotiateCodec()
    expect(result.supported).toBe(true)
    expect(result.mimeType).toBe('audio/webm')
    expect(result.codecString).toBe('audio/webm;codecs=opus')
  })

  it('falls back to MP4/AAC when WebM unsupported', () => {
    vi.stubGlobal('MediaRecorder', {
      isTypeSupported: (type: string) => type === 'audio/mp4;codecs=mp4a.40.2',
    })
    const result = negotiateCodec()
    expect(result.supported).toBe(true)
    expect(result.mimeType).toBe('audio/mp4')
    expect(result.codecString).toBe('audio/mp4;codecs=mp4a.40.2')
  })

  it('returns supported=false when no codecs supported', () => {
    vi.stubGlobal('MediaRecorder', {
      isTypeSupported: () => false,
    })
    const result = negotiateCodec()
    expect(result.supported).toBe(false)
    expect(result.mimeType).toBeNull()
  })
})

describe('useMediaRecorder', () => {
  const onDataAvailable = vi.fn()
  const onError = vi.fn()
  const onAutoStop = vi.fn()
  const onCaptureEnded = vi.fn()
  const originalMediaRecorder = globalThis.MediaRecorder
  const originalAudioContext = globalThis.AudioContext

  type FakeTrack = {
    kind: string
    readyState: 'live' | 'ended'
    muted: boolean
    stop: ReturnType<typeof vi.fn>
    addEventListener: ReturnType<typeof vi.fn>
    emitEnded: () => void
    emitMuted: () => void
    emitUnmuted: () => void
  }

  type FakeStream = {
    getTracks: () => FakeTrack[]
    getAudioTracks: () => FakeTrack[]
    getVideoTracks: () => FakeTrack[]
  }

  class FakeMediaRecorder {
    static instances: FakeMediaRecorder[] = []
    static isTypeSupported(type: string) {
      return type === 'audio/webm;codecs=opus'
    }

    state: RecordingState = 'inactive'
    ondataavailable: ((event: BlobEvent) => void) | null = null
    onerror: (() => void) | null = null
    onstop: (() => void) | null = null

    constructor(
      public stream: FakeStream,
      public options: MediaRecorderOptions,
    ) {
      FakeMediaRecorder.instances.push(this)
    }

    start = vi.fn(() => {
      this.state = 'recording'
    })
    pause = vi.fn(() => {
      this.state = 'paused'
    })
    resume = vi.fn(() => {
      this.state = 'recording'
    })
    stop = vi.fn(() => {
      this.state = 'inactive'
      this.onstop?.()
    })
  }

  function makeTrack(kind: string): FakeTrack {
    const listeners = new Map<string, EventListener[]>()
    const emit = (event: string) => {
      for (const listener of listeners.get(event) ?? []) {
        listener(new Event(event))
      }
    }
    const track: FakeTrack = {
      kind,
      readyState: 'live',
      muted: false,
      stop: vi.fn(),
      addEventListener: vi.fn((event: string, listener: EventListener) => {
        listeners.set(event, [...(listeners.get(event) ?? []), listener])
      }),
      emitEnded: () => {
        track.readyState = 'ended'
        emit('ended')
      },
      emitMuted: () => {
        track.muted = true
        emit('mute')
      },
      emitUnmuted: () => {
        track.muted = false
        emit('unmute')
      },
    }
    return track
  }

  function makeStream(tracks: FakeTrack[]): FakeStream {
    return {
      getTracks: () => tracks,
      getAudioTracks: () => tracks.filter((track) => track.kind === 'audio'),
      getVideoTracks: () => tracks.filter((track) => track.kind === 'video'),
    }
  }

  function installRecorderMocks(
    audioContextState: AudioContextState = 'running',
    opts: { resumeStaysSuspended?: boolean } = {},
  ) {
    FakeMediaRecorder.instances = []
    vi.stubGlobal('MediaRecorder', FakeMediaRecorder)

    const destinationStream = makeStream([makeTrack('audio')])
    const resume = vi.fn(async function (this: { state: AudioContextState }) {
      if (!opts.resumeStaysSuspended) {
        this.state = 'running'
      }
    })
    const close = vi.fn(async () => undefined)
    const connect = vi.fn()
    const createGain = vi.fn(() => ({ gain: { value: 1 }, connect }))
    const compressor = {
      threshold: { value: 0 },
      knee: { value: 0 },
      ratio: { value: 0 },
      attack: { value: 0 },
      release: { value: 0 },
      connect,
    }

    class FakeAudioContext {
      state = audioContextState
      resume = resume.bind(this)
      close = close
      createMediaStreamSource = vi.fn(() => ({ connect }))
      createAnalyser = vi.fn(() => ({ fftSize: 0, connect }))
      createGain = createGain
      createDynamicsCompressor = vi.fn(() => compressor)
      createMediaStreamDestination = vi.fn(() => ({ stream: destinationStream }))
    }

    vi.stubGlobal('AudioContext', FakeAudioContext)
    return { destinationStream, resume, close, createGain, compressor }
  }

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    if (originalMediaRecorder) {
      vi.stubGlobal('MediaRecorder', originalMediaRecorder)
    } else {
      vi.unstubAllGlobals()
    }
    if (originalAudioContext) {
      vi.stubGlobal('AudioContext', originalAudioContext)
    }
  })

  it('returns isSupported=false when MediaRecorder is undefined', () => {
    vi.stubGlobal('MediaRecorder', undefined)

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    expect(result.current.isSupported).toBe(false)
    expect(result.current.mimeType).toBeNull()
    expect(result.current.isRecording).toBe(false)
    expect(result.current.isPaused).toBe(false)
  })

  it('returns isSupported=true with correct mimeType when WebM/Opus supported', () => {
    vi.stubGlobal('MediaRecorder', class {
      static isTypeSupported(type: string) {
        return type === 'audio/webm;codecs=opus'
      }
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    expect(result.current.isSupported).toBe(true)
    expect(result.current.mimeType).toBe('audio/webm')
  })

  it('start() calls onError when unsupported', async () => {
    vi.stubGlobal('MediaRecorder', undefined)

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    await expect(
      act(async () => {
        await result.current.start()
      }),
    ).rejects.toThrow()

    expect(onError).toHaveBeenCalledWith('Recording is not supported in this browser.')
  })

  it('exposes prepare, start, stop, pause, resume, and discardPrepared functions', () => {
    vi.stubGlobal('MediaRecorder', class {
      static isTypeSupported(type: string) {
        return type === 'audio/webm;codecs=opus'
      }
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    expect(typeof result.current.prepare).toBe('function')
    expect(typeof result.current.start).toBe('function')
    expect(typeof result.current.stop).toBe('function')
    expect(typeof result.current.pause).toBe('function')
    expect(typeof result.current.resume).toBe('function')
    expect(typeof result.current.discardPrepared).toBe('function')
  })

  it('prepare() creates a mic-only recorder without starting it', async () => {
    installRecorderMocks()
    const micStream = makeStream([makeTrack('audio')])
    const getUserMedia = vi.fn().mockResolvedValue(micStream)
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: 'default',
        captureSource: 'microphone',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    await act(async () => {
      await result.current.prepare()
    })

    expect(getUserMedia).toHaveBeenCalledWith({
      audio: { deviceId: { exact: 'default' } },
    })
    expect(FakeMediaRecorder.instances).toHaveLength(1)
    expect(FakeMediaRecorder.instances[0].start).not.toHaveBeenCalled()
  })

  it('mixed prepare() asks for display capture before microphone capture', async () => {
    installRecorderMocks()
    const calls: string[] = []
    const displayStream = makeStream([makeTrack('video'), makeTrack('audio')])
    const micStream = makeStream([makeTrack('audio')])
    const getDisplayMedia = vi.fn(async () => {
      calls.push('display')
      return displayStream
    })
    const getUserMedia = vi.fn(async () => {
      calls.push('mic')
      return micStream
    })
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getDisplayMedia, getUserMedia },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: 'default',
        captureSource: 'mixed_audio',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    await act(async () => {
      await result.current.prepare()
    })

    expect(calls).toEqual(['display', 'mic'])
  })

  it('mixed prepare() records display audio without requiring a microphone', async () => {
    installRecorderMocks()
    const displayStream = makeStream([makeTrack('video'), makeTrack('audio')])
    const getUserMedia = vi.fn()
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getDisplayMedia: vi.fn().mockResolvedValue(displayStream), getUserMedia },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        captureSource: 'mixed_audio',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    await act(async () => {
      await result.current.prepare()
      await result.current.start()
    })

    expect(getUserMedia).not.toHaveBeenCalled()
    expect(FakeMediaRecorder.instances[0].start).toHaveBeenCalled()
  })

  it('keeps meeting capture alive when the optional microphone ends', async () => {
    installRecorderMocks()
    const displayAudio = makeTrack('audio')
    const micAudio = makeTrack('audio')
    const onMicrophoneUnavailable = vi.fn()
    Object.defineProperty(navigator, 'mediaDevices', {
      value: {
        getDisplayMedia: vi.fn().mockResolvedValue(makeStream([makeTrack('video'), displayAudio])),
        getUserMedia: vi.fn().mockResolvedValue(makeStream([micAudio])),
      },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: 'default',
        captureSource: 'mixed_audio',
        onDataAvailable,
        onError,
        onAutoStop,
        onCaptureEnded,
        onMicrophoneUnavailable,
      }),
    )

    await act(async () => {
      await result.current.prepare()
    })
    micAudio.emitEnded()

    expect(onMicrophoneUnavailable).toHaveBeenCalledTimes(1)
    expect(onCaptureEnded).not.toHaveBeenCalled()
    expect(result.current.isPreparedCaptureLive()).toBe(true)
  })

  it('warns when a meeting audio track is muted', async () => {
    installRecorderMocks()
    const displayAudio = makeTrack('audio')
    const onCaptureMuted = vi.fn()
    Object.defineProperty(navigator, 'mediaDevices', {
      value: {
        getDisplayMedia: vi.fn().mockResolvedValue(makeStream([makeTrack('video'), displayAudio])),
        getUserMedia: vi.fn().mockResolvedValue(makeStream([makeTrack('audio')])),
      },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: 'default',
        captureSource: 'mixed_audio',
        onDataAvailable,
        onError,
        onAutoStop,
        onCaptureMuted,
      }),
    )

    await act(async () => {
      await result.current.prepare()
    })
    displayAudio.emitMuted()

    expect(onCaptureMuted).toHaveBeenCalledWith('meeting')
  })

  it('refuses to start when the selected display audio ended after preparation', async () => {
    installRecorderMocks()
    const displayAudio = makeTrack('audio')
    Object.defineProperty(navigator, 'mediaDevices', {
      value: {
        getDisplayMedia: vi.fn().mockResolvedValue(makeStream([makeTrack('video'), displayAudio])),
        getUserMedia: vi.fn().mockResolvedValue(makeStream([makeTrack('audio')])),
      },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: 'default',
        captureSource: 'mixed_audio',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )
    await act(async () => {
      await result.current.prepare()
    })
    displayAudio.emitEnded()

    expect(result.current.isPreparedCaptureLive()).toBe(false)
    await expect(result.current.start()).rejects.toThrow('display capture ended')
    expect(FakeMediaRecorder.instances[0].start).not.toHaveBeenCalled()
  })

  it('applies headroom and a compressor when mixing microphone and meeting audio', async () => {
    const { createGain, compressor } = installRecorderMocks()
    Object.defineProperty(navigator, 'mediaDevices', {
      value: {
        getDisplayMedia: vi.fn().mockResolvedValue(makeStream([makeTrack('video'), makeTrack('audio')])),
        getUserMedia: vi.fn().mockResolvedValue(makeStream([makeTrack('audio')])),
      },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: 'default',
        captureSource: 'mixed_audio',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )
    await act(async () => {
      await result.current.prepare()
    })

    // The first gain is the mix bus; each input receives half-gain headroom.
    expect(createGain).toHaveBeenCalledTimes(3)
    expect(createGain.mock.results.map((result) => result.value.gain.value)).toEqual([1, 0.5, 0.5])
    expect(compressor.threshold.value).toBe(-12)
    expect(compressor.ratio.value).toBe(12)
  })

  it('mixed prepare() rejects and stops display video when no display audio track is returned', async () => {
    installRecorderMocks()
    const displayVideo = makeTrack('video')
    const displayStream = makeStream([displayVideo])
    const getDisplayMedia = vi.fn().mockResolvedValue(displayStream)
    const getUserMedia = vi.fn()
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getDisplayMedia, getUserMedia },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        captureSource: 'mixed_audio',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    await expect(
      act(async () => {
        await result.current.prepare()
      }),
    ).rejects.toThrow('display audio unavailable')

    expect(displayVideo.stop).toHaveBeenCalled()
    expect(getUserMedia).not.toHaveBeenCalled()
    await waitFor(() => {
      expect(onError).toHaveBeenCalledWith('No shared audio was detected. Chrome or Edge tab audio works reliably; full-screen, native-app, and system audio depend on your browser and operating system.')
    })
  })

  it('reports display permission denial separately from missing shared audio', async () => {
    installRecorderMocks()
    Object.defineProperty(navigator, 'mediaDevices', {
      value: {
        getDisplayMedia: vi.fn().mockRejectedValue(new DOMException('Denied', 'NotAllowedError')),
        getUserMedia: vi.fn(),
      },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: 'default',
        captureSource: 'mixed_audio',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    await expect(result.current.prepare()).rejects.toThrow('display capture denied')
    expect(onError).toHaveBeenCalledWith(
      'Screen or tab sharing was cancelled or denied. Choose the meeting source and enable audio sharing.',
    )
  })

  it('prepare() resumes a suspended AudioContext before start()', async () => {
    const { resume } = installRecorderMocks('suspended')
    const micStream = makeStream([makeTrack('audio')])
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia: vi.fn().mockResolvedValue(micStream) },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        captureSource: 'microphone',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    await act(async () => {
      await result.current.prepare()
    })

    expect(resume).toHaveBeenCalled()
  })

  it('mixed source ending calls onCaptureEnded', async () => {
    installRecorderMocks()
    const displayAudio = makeTrack('audio')
    const displayStream = makeStream([makeTrack('video'), displayAudio])
    const micStream = makeStream([makeTrack('audio')])
    Object.defineProperty(navigator, 'mediaDevices', {
      value: {
        getDisplayMedia: vi.fn().mockResolvedValue(displayStream),
        getUserMedia: vi.fn().mockResolvedValue(micStream),
      },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        captureSource: 'mixed_audio',
        onDataAvailable,
        onError,
        onAutoStop,
        onCaptureEnded,
      }),
    )

    await act(async () => {
      await result.current.prepare()
    })
    displayAudio.emitEnded()

    expect(onCaptureEnded).toHaveBeenCalledTimes(1)
  })

  it('does not end meeting capture when only the display video track ends', async () => {
    installRecorderMocks()
    const displayVideo = makeTrack('video')
    const displayAudio = makeTrack('audio')
    Object.defineProperty(navigator, 'mediaDevices', {
      value: {
        getDisplayMedia: vi.fn().mockResolvedValue(makeStream([displayVideo, displayAudio])),
        getUserMedia: vi.fn().mockResolvedValue(makeStream([makeTrack('audio')])),
      },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: 'default',
        captureSource: 'mixed_audio',
        onDataAvailable,
        onError,
        onAutoStop,
        onCaptureEnded,
      }),
    )
    await act(async () => {
      await result.current.prepare()
    })
    displayVideo.emitEnded()

    expect(onCaptureEnded).not.toHaveBeenCalled()
    expect(result.current.isPreparedCaptureLive()).toBe(true)
  })

  it('fires onCaptureEnded at most once when multiple display tracks end', async () => {
    installRecorderMocks()
    const displayVideo = makeTrack('video')
    const displayAudio = makeTrack('audio')
    const displayStream = makeStream([displayVideo, displayAudio])
    const micStream = makeStream([makeTrack('audio')])
    Object.defineProperty(navigator, 'mediaDevices', {
      value: {
        getDisplayMedia: vi.fn().mockResolvedValue(displayStream),
        getUserMedia: vi.fn().mockResolvedValue(micStream),
      },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        captureSource: 'mixed_audio',
        onDataAvailable,
        onError,
        onAutoStop,
        onCaptureEnded,
      }),
    )

    await act(async () => {
      await result.current.prepare()
    })
    // Browser "Stop sharing" fires `ended` on every track of the display stream.
    displayVideo.emitEnded()
    displayAudio.emitEnded()

    expect(onCaptureEnded).toHaveBeenCalledTimes(1)
  })

  it('re-arms onCaptureEnded when a new session is prepared', async () => {
    installRecorderMocks()
    const firstVideo = makeTrack('video')
    const firstAudio = makeTrack('audio')
    const secondVideo = makeTrack('video')
    const secondAudio = makeTrack('audio')
    const getDisplayMedia = vi
      .fn()
      .mockResolvedValueOnce(makeStream([firstVideo, firstAudio]))
      .mockResolvedValueOnce(makeStream([secondVideo, secondAudio]))
    const getUserMedia = vi.fn(async () => makeStream([makeTrack('audio')]))
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getDisplayMedia, getUserMedia },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        captureSource: 'mixed_audio',
        onDataAvailable,
        onError,
        onAutoStop,
        onCaptureEnded,
      }),
    )

    await act(async () => {
      await result.current.prepare()
    })
    firstVideo.emitEnded()
    firstAudio.emitEnded()
    expect(onCaptureEnded).toHaveBeenCalledTimes(1)

    await act(async () => {
      await result.current.prepare()
    })
    secondVideo.emitEnded()
    secondAudio.emitEnded()

    expect(onCaptureEnded).toHaveBeenCalledTimes(2)
  })

  it('mic prepare() records the raw stream when the AudioContext stays suspended', async () => {
    const { close } = installRecorderMocks('suspended', { resumeStaysSuspended: true })
    const micStream = makeStream([makeTrack('audio')])
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia: vi.fn().mockResolvedValue(micStream) },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        captureSource: 'microphone',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    await act(async () => {
      await result.current.prepare()
    })

    // Recording proceeds on the raw getUserMedia stream; only the waveform degrades.
    expect(FakeMediaRecorder.instances).toHaveLength(1)
    expect(FakeMediaRecorder.instances[0].stream).toBe(micStream)
    expect(result.current.analyserNode).toBeNull()
    expect(onError).not.toHaveBeenCalled()
    expect(close).toHaveBeenCalled()

    await act(async () => {
      await result.current.start()
    })
    expect(FakeMediaRecorder.instances[0].start).toHaveBeenCalled()
    expect(result.current.isRecording).toBe(true)
  })

  it('mixed prepare() rejects when the AudioContext stays suspended', async () => {
    installRecorderMocks('suspended', { resumeStaysSuspended: true })
    const displayStream = makeStream([makeTrack('video'), makeTrack('audio')])
    const micStream = makeStream([makeTrack('audio')])
    Object.defineProperty(navigator, 'mediaDevices', {
      value: {
        getDisplayMedia: vi.fn().mockResolvedValue(displayStream),
        getUserMedia: vi.fn().mockResolvedValue(micStream),
      },
      configurable: true,
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        captureSource: 'mixed_audio',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    let caught: unknown = null
    await act(async () => {
      try {
        await result.current.prepare()
      } catch (e) {
        caught = e
      }
    })
    expect((caught as Error | null)?.message).toBe('audio context suspended')

    expect(onError).toHaveBeenCalledWith(
      'Audio capture could not start. Click Start Recording again and keep the browser tab active.',
    )
    expect(FakeMediaRecorder.instances).toHaveLength(0)
  })

  it('auto-stop delegates to the caller instead of stopping the native recorder', async () => {
    vi.useFakeTimers()
    installRecorderMocks()
    const micStream = makeStream([makeTrack('audio')])
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia: vi.fn().mockResolvedValue(micStream) },
      configurable: true,
    })
    const localOnAutoStop = vi.fn()

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        captureSource: 'microphone',
        onDataAvailable,
        onError,
        onAutoStop: localOnAutoStop,
      }),
    )

    await act(async () => {
      await result.current.prepare()
    })
    await act(async () => {
      await result.current.start()
    })

    const recorderInstance = FakeMediaRecorder.instances[0]
    await act(async () => {
      await vi.advanceTimersByTimeAsync(8 * 60 * 60 * 1000)
    })

    expect(localOnAutoStop).toHaveBeenCalledTimes(1)
    // Stopping natively here would make the caller's stop() see 'inactive' and
    // resolve without waiting for the final chunk write.
    expect(recorderInstance.stop).not.toHaveBeenCalled()
    expect(recorderInstance.state).toBe('recording')
    vi.useRealTimers()
  })

  it('stop() waits for the final chunk write before resolving', async () => {
    installRecorderMocks()
    const micStream = makeStream([makeTrack('audio')])
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia: vi.fn().mockResolvedValue(micStream) },
      configurable: true,
    })

    let releaseWrite: () => void = () => {}
    const localOnDataAvailable = vi.fn(
      () => new Promise<void>((resolve) => { releaseWrite = resolve }),
    )

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        captureSource: 'microphone',
        onDataAvailable: localOnDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    await act(async () => {
      await result.current.prepare()
    })
    await act(async () => {
      await result.current.start()
    })

    // Real MediaRecorder flushes a final ondataavailable before onstop.
    const recorderInstance = FakeMediaRecorder.instances[0]
    recorderInstance.stop = vi.fn(() => {
      recorderInstance.state = 'inactive'
      recorderInstance.ondataavailable?.({
        data: new Blob(['tail'], { type: 'audio/webm' }),
      } as BlobEvent)
      recorderInstance.onstop?.()
    })

    let stopSettled = false
    const stopPromise = result.current.stop().then(() => {
      stopSettled = true
    })

    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(localOnDataAvailable).toHaveBeenCalledTimes(1)
    expect(stopSettled).toBe(false)

    await act(async () => {
      releaseWrite()
      await stopPromise
    })
    expect(stopSettled).toBe(true)
  })

  it('serializes chunk admission so a later sequence cannot overtake IndexedDB persistence', async () => {
    installRecorderMocks()
    const micStream = makeStream([makeTrack('audio')])
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia: vi.fn().mockResolvedValue(micStream) },
      configurable: true,
    })
    let releaseFirst: () => void = () => {}
    const localOnDataAvailable = vi.fn((_: Blob, seq: number) => {
      if (seq !== 0) return Promise.resolve()
      return new Promise<void>((resolve) => { releaseFirst = resolve })
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: 'default',
        captureSource: 'microphone',
        onDataAvailable: localOnDataAvailable,
        onError,
        onAutoStop,
      }),
    )
    await act(async () => {
      await result.current.prepare()
      await result.current.start()
    })

    const recorderInstance = FakeMediaRecorder.instances[0]
    recorderInstance.ondataavailable?.({ data: new Blob(['first']) } as BlobEvent)
    recorderInstance.ondataavailable?.({ data: new Blob(['second']) } as BlobEvent)

    await waitFor(() => {
      expect(localOnDataAvailable).toHaveBeenCalledWith(expect.any(Blob), 0)
    })
    expect(localOnDataAvailable).toHaveBeenCalledTimes(1)

    releaseFirst()
    await waitFor(() => {
      expect(localOnDataAvailable).toHaveBeenCalledWith(expect.any(Blob), 1)
    })
  })

  it('preserves the final buffered chunk before reporting a native recorder interruption', async () => {
    installRecorderMocks()
    const micStream = makeStream([makeTrack('audio')])
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia: vi.fn().mockResolvedValue(micStream) },
      configurable: true,
    })
    let releaseWrite: () => void = () => {}
    const localOnDataAvailable = vi.fn(
      () => new Promise<void>((resolve) => { releaseWrite = resolve }),
    )
    const onCaptureInterrupted = vi.fn()

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: 'default',
        captureSource: 'microphone',
        onDataAvailable: localOnDataAvailable,
        onError,
        onAutoStop,
        onCaptureInterrupted,
      }),
    )
    await act(async () => {
      await result.current.prepare()
      await result.current.start()
    })

    const recorderInstance = FakeMediaRecorder.instances[0]
    recorderInstance.onerror?.()
    recorderInstance.ondataavailable?.({ data: new Blob(['tail']) } as BlobEvent)
    recorderInstance.state = 'inactive'
    recorderInstance.onstop?.()

    await waitFor(() => {
      expect(localOnDataAvailable).toHaveBeenCalledTimes(1)
    })
    expect(onCaptureInterrupted).not.toHaveBeenCalled()

    releaseWrite()
    await waitFor(() => {
      expect(onCaptureInterrupted).toHaveBeenCalledWith('Recording error. The recording has been stopped.')
    })
  })

  it('splits a delayed 30 MiB blob into sequential server-safe chunks', async () => {
    installRecorderMocks()
    const micStream = makeStream([makeTrack('audio')])
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia: vi.fn().mockResolvedValue(micStream) },
      configurable: true,
    })
    const localOnDataAvailable = vi.fn()

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: 'default',
        captureSource: 'microphone',
        onDataAvailable: localOnDataAvailable,
        onError,
        onAutoStop,
      }),
    )
    await act(async () => {
      await result.current.prepare()
      await result.current.start()
    })

    const recorderInstance = FakeMediaRecorder.instances[0]
    const delayedBlob = {
      size: 30 * 1024 * 1024,
      type: 'audio/webm',
      slice: vi.fn(() => new Blob(['part'], { type: 'audio/webm' })),
    } as unknown as Blob
    recorderInstance.ondataavailable?.({ data: delayedBlob } as BlobEvent)

    await waitFor(() => {
      expect(localOnDataAvailable).toHaveBeenCalledTimes(2)
    })
    expect(localOnDataAvailable.mock.calls.map((call) => call[1])).toEqual([0, 1])
    expect(delayedBlob.slice).toHaveBeenNthCalledWith(1, 0, MAX_RECORDING_CHUNK_BYTES, 'audio/webm')
    expect(delayedBlob.slice).toHaveBeenNthCalledWith(2, MAX_RECORDING_CHUNK_BYTES, 30 * 1024 * 1024, 'audio/webm')
  })

  it('analyserNode is null when not recording', () => {
    vi.stubGlobal('MediaRecorder', class {
      static isTypeSupported(type: string) {
        return type === 'audio/webm;codecs=opus'
      }
    })

    const { result } = renderHook(() =>
      useMediaRecorder({
        deviceId: '',
        onDataAvailable,
        onError,
        onAutoStop,
      }),
    )

    expect(result.current.analyserNode).toBeNull()
  })

  it('uses 8-hour max duration default when env is missing', () => {
    expect(getRecordingMaxDurationMs(undefined)).toBe(8 * 60 * 60 * 1000)
  })

  it('uses env override for max duration when valid', () => {
    expect(getRecordingMaxDurationMs('3600000')).toBe(3600000)
  })

  it('falls back to default max duration when env is invalid', () => {
    expect(getRecordingMaxDurationMs('not-a-number')).toBe(8 * 60 * 60 * 1000)
    expect(getRecordingMaxDurationMs('0')).toBe(8 * 60 * 60 * 1000)
    expect(getRecordingMaxDurationMs('-100')).toBe(8 * 60 * 60 * 1000)
  })
})
