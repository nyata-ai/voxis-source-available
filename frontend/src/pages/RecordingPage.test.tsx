import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import type { Location } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ApiError } from '@/lib/api-client'
import { shouldBlockRecordingExit } from '@/lib/recording-exit-guard'
import { RecordingPage } from './RecordingPage'
import { isHeartbeatCadenceSafe } from '@/lib/recording-exit-guard'
import { toast } from '@/lib/toast'

// --- Mocks ---

const mockNavigate = vi.fn()
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

// Mock the recording store
const mockStore = {
  sessionId: null as string | null,
  status: 'idle' as string,
  elapsed: 0,
  mimeType: null as string | null,
  captureSource: 'microphone' as 'microphone' | 'mixed_audio',
  outboxSize: 0,
  lastSavedAt: null as number | null,
  errorMessage: null as string | null,
  localWriteFailed: false,
  flushFailure: null as { status: number; seq: number; code?: 'storage_quota_exceeded' } | null,
  setSessionId: vi.fn(),
  setStatus: vi.fn(),
  setCaptureSource: vi.fn((source: 'microphone' | 'mixed_audio') => {
    mockStore.captureSource = source
  }),
  setElapsed: vi.fn(),
  incrementElapsed: vi.fn(),
  setMimeType: vi.fn(),
  setOutboxSize: vi.fn(),
  setLastSavedAt: vi.fn(),
  setLocalWriteFailed: vi.fn((failed: boolean) => {
    mockStore.localWriteFailed = failed
  }),
  setFlushFailure: vi.fn((failure: { status: number; seq: number; code?: 'storage_quota_exceeded' } | null) => {
    mockStore.flushFailure = failure
  }),
  setError: vi.fn(),
  reset: vi.fn(),
}

// Model real Zustand semantics: each render receives an immutable snapshot
// (stale in later closures), while getState() always returns live state.
vi.mock('@/stores/recording', () => ({
  useRecordingStore: Object.assign(() => ({ ...mockStore }), {
    getState: () => mockStore,
  }),
}))

// Mock toast
vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn(), loading: vi.fn(), dismiss: vi.fn() },
}))

// Mock chunk-outbox
vi.mock('@/lib/chunk-outbox', () => ({
  cleanupOldEntries: vi.fn().mockResolvedValue(0),
  putChunk: vi.fn().mockResolvedValue(1),
  getOldestUnsent: vi.fn().mockResolvedValue(null),
  getAllBySession: vi.fn().mockResolvedValue([]),
  deleteChunk: vi.fn().mockResolvedValue(undefined),
  countBySession: vi.fn().mockResolvedValue(0),
  deleteBySession: vi.fn().mockResolvedValue(undefined),
}))

// Mock hooks
const mockRefetchInterrupted = vi.fn().mockResolvedValue({ data: { items: [] } })
const mockInterruptedData = { data: { items: [] as Array<unknown> }, isLoading: false, refetch: mockRefetchInterrupted }
vi.mock('@/hooks/useInterruptedRecording', () => ({
  useInterruptedRecording: () => mockInterruptedData,
}))

const { mockUseUsageStats, mockRefetchUsage } = vi.hoisted(() => ({
  mockUseUsageStats: vi.fn(),
  mockRefetchUsage: vi.fn(),
}))
vi.mock('@/hooks/useUsageStats', () => ({
  useUsageStats: mockUseUsageStats,
}))

const mockMicDevices = {
  devices: [{ deviceId: 'default', label: 'Built-in Microphone' }],
  selectedDeviceId: 'default',
  setSelectedDeviceId: vi.fn(),
  error: null as string | null,
  isLoading: false,
}
vi.mock('@/hooks/useMicrophoneDevices', () => ({
  useMicrophoneDevices: () => mockMicDevices,
}))

const mockMediaRecorder = {
  isSupported: true,
  mimeType: 'audio/webm' as const,
  prepare: vi.fn(),
  start: vi.fn(),
  discardPrepared: vi.fn(),
  stop: vi.fn(),
  pause: vi.fn(),
  resume: vi.fn(),
  isRecording: false,
  isPaused: false,
  analyserNode: null,
  isPreparedCaptureLive: vi.fn(() => true),
}

type MockMediaRecorderOptions = {
  onDataAvailable?: (blob: Blob, seq: number) => unknown
  onCaptureEnded?: () => void
  onCaptureInterrupted?: (error: string) => void
  onCaptureMuted?: (source: 'microphone' | 'meeting') => void
  onMicrophoneUnavailable?: () => void
  onAutoStop?: () => void
}
let capturedMediaRecorderOptions: MockMediaRecorderOptions | null = null

vi.mock('@/hooks/useMediaRecorder', () => ({
  useMediaRecorder: (options: MockMediaRecorderOptions) => {
    capturedMediaRecorderOptions = options
    return mockMediaRecorder
  },
  negotiateCodec: () => ({
    supported: true,
    mimeType: 'audio/webm',
    codecString: 'audio/webm;codecs=opus',
  }),
}))

const mockChunkUpload = {
  enqueue: vi.fn(),
  waitForDrain: vi.fn(),
  retryFlush: vi.fn(),
  clearSession: vi.fn(),
}

vi.mock('@/hooks/useChunkUpload', () => ({
  useChunkUpload: () => mockChunkUpload,
}))

const mockCreateSession = { mutateAsync: vi.fn(), isPending: false }
const mockCompleteSession = { mutateAsync: vi.fn(), isPending: false }
const mockPauseSession = { mutate: vi.fn(), isPending: false }
const mockResumeSession = { mutate: vi.fn(), isPending: false }
const mockRecoverSession = { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }
const mockReleaseSession = { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }
const mockAbandonSession = { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }
const mockSendHeartbeat = vi.fn()
const mockFetchActiveSession = vi.fn()
const mockUploadChunk = vi.fn()

vi.mock('@/hooks/useRecordingSession', () => ({
  useCreateRecordingSession: () => mockCreateSession,
  useCompleteRecordingSession: () => mockCompleteSession,
  usePauseRecordingSession: () => mockPauseSession,
  useResumeRecordingSession: () => mockResumeSession,
  useRecoverRecordingSession: () => mockRecoverSession,
  useReleaseRecordingSession: () => mockReleaseSession,
  useAbandonRecordingSession: () => mockAbandonSession,
  sendHeartbeat: (...args: unknown[]) => mockSendHeartbeat(...args),
  fetchActiveRecordingSession: (...args: unknown[]) => mockFetchActiveSession(...args),
  uploadChunk: (...args: unknown[]) => mockUploadChunk(...args),
  recordingKeys: {
    all: ['recordings'],
    interrupted: () => ['recordings', 'interrupted'],
    detail: (id: string) => ['recordings', 'detail', id],
  },
}))

// Mock BrowserCompatCheck as a passthrough — browser API detection is tested separately
vi.mock('@/components/recording/BrowserCompatCheck', () => ({
  BrowserCompatCheck: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

// --- Helpers ---

// The recording page relies on useBlocker, which only exists inside a data
// router — so tests mount it through createMemoryRouter, like production.
function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })

  const router = createMemoryRouter(
    [
      { path: '/record', element: <RecordingPage /> },
      { path: '/media', element: <div>Media Library</div> },
    ],
    { initialEntries: ['/record'] },
  )

  const tree = (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )

  const utils = render(tree)
  return { ...utils, router, rerenderPage: () => utils.rerender(tree) }
}

function loc(pathname: string): Location {
  return { pathname, search: '', hash: '', state: null, key: 'test' }
}

/** Let handleStop's `setStatus('completing')` actually move the mocked store. */
function trackStatusOnce() {
  mockStore.setStatus.mockImplementationOnce((status: string) => {
    mockStore.status = status
  })
}

// --- Tests ---

describe('RecordingPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockNavigate.mockReset()
    // Reset store to idle state
    mockStore.sessionId = null
    mockStore.status = 'idle'
    mockStore.elapsed = 0
    mockStore.mimeType = null
    mockStore.captureSource = 'microphone'
    mockStore.outboxSize = 0
    mockStore.lastSavedAt = null
    mockStore.errorMessage = null
    mockStore.localWriteFailed = false
    mockStore.flushFailure = null
    capturedMediaRecorderOptions = null

    // Reset mic devices
    mockMicDevices.devices = [{ deviceId: 'default', label: 'Built-in Microphone' }]
    mockMicDevices.error = null
    mockMicDevices.isLoading = false

    // Reset interrupted
    mockInterruptedData.data = { items: [] as Array<unknown> }
    mockRefetchInterrupted.mockReset()
    mockRefetchInterrupted.mockResolvedValue({ data: { items: [] } })
    mockSendHeartbeat.mockResolvedValue(undefined)
    mockFetchActiveSession.mockResolvedValue(null)
    mockUploadChunk.mockResolvedValue(undefined)
    mockRefetchUsage.mockReset()
    mockRefetchUsage.mockResolvedValue({ data: undefined })
    mockUseUsageStats.mockReturnValue({ data: undefined, refetch: mockRefetchUsage })
    // mockReset also drops unconsumed mock*Once queues that clearAllMocks keeps.
    mockCompleteSession.mutateAsync.mockReset()
    mockCompleteSession.mutateAsync.mockResolvedValue({ status: 'completing' })
    mockStore.setStatus.mockReset()
    mockChunkUpload.enqueue.mockReset()
    mockChunkUpload.enqueue.mockResolvedValue(undefined)
    mockChunkUpload.waitForDrain.mockReset()
    mockChunkUpload.waitForDrain.mockResolvedValue(undefined)
    mockChunkUpload.retryFlush.mockReset()
    mockChunkUpload.retryFlush.mockResolvedValue(undefined)
    mockChunkUpload.clearSession.mockReset()
    mockChunkUpload.clearSession.mockResolvedValue(undefined)
    mockReleaseSession.mutateAsync.mockReset()
    mockReleaseSession.mutateAsync.mockResolvedValue({ status: 'interrupted' })
    mockAbandonSession.mutateAsync.mockReset()
    mockAbandonSession.mutateAsync.mockResolvedValue(undefined)
    mockCreateSession.mutateAsync.mockReset()
    mockMediaRecorder.prepare.mockResolvedValue(undefined)
    mockMediaRecorder.start.mockResolvedValue(undefined)
    mockMediaRecorder.discardPrepared.mockResolvedValue(undefined)
    mockMediaRecorder.stop.mockResolvedValue(undefined)
    mockMediaRecorder.isPreparedCaptureLive.mockReset()
    mockMediaRecorder.isPreparedCaptureLive.mockReturnValue(true)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  // State 1: Idle / Pre-recording
  it('renders "Ready to record" heading in idle state', () => {
    renderPage()
    expect(screen.getByText('Ready to record')).toBeInTheDocument()
  })

  it('renders "Start Recording" button in idle state', () => {
    renderPage()
    expect(screen.getByRole('button', { name: /start recording/i })).toBeInTheDocument()
  })

  it('renders microphone label in idle state', () => {
    renderPage()
    expect(screen.getByText('Built-in Microphone')).toBeInTheDocument()
  })

  it('disables start button when mic is loading', () => {
    mockMicDevices.isLoading = true
    renderPage()
    expect(screen.getByRole('button', { name: /start recording/i })).toBeDisabled()
  })

  it('shows mic error message', () => {
    mockMicDevices.error = 'Microphone access denied.'
    renderPage()
    expect(screen.getByText('Microphone access denied.')).toBeInTheDocument()
  })

  it('disables start button when mic error exists', () => {
    mockMicDevices.error = 'Microphone access denied.'
    renderPage()
    expect(screen.getByRole('button', { name: /start recording/i })).toBeDisabled()
  })

  // State 2: Recording active
  it('renders "Recording" label and controls when recording', () => {
    mockStore.status = 'recording'
    renderPage()
    expect(screen.getByText('Recording')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /pause/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
  })

  it('renders timer when recording', () => {
    mockStore.status = 'recording'
    mockStore.elapsed = 90
    renderPage()
    expect(screen.getByRole('timer')).toBeInTheDocument()
  })

  // A sub-second capture stitches below the backend's 1024-byte media minimum
  // and can never be saved — Stop stays disabled until the recording is viable.
  it('disables Stop with a hint until the minimum duration is reached', () => {
    mockStore.status = 'recording'
    mockStore.elapsed = 1
    renderPage()
    expect(screen.getByRole('button', { name: /stop/i })).toBeDisabled()
    expect(
      screen.getByText('Record for at least 2 seconds before stopping'),
    ).toBeInTheDocument()
  })

  it('enables Stop and hides the hint once the minimum duration is reached', () => {
    mockStore.status = 'recording'
    mockStore.elapsed = 2
    renderPage()
    expect(screen.getByRole('button', { name: /stop/i })).toBeEnabled()
    expect(
      screen.queryByText('Record for at least 2 seconds before stopping'),
    ).not.toBeInTheDocument()
  })

  // State 2: Paused
  it('renders "Paused" label and resume button when paused', () => {
    mockStore.status = 'paused'
    renderPage()
    expect(screen.getByText('Paused')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /resume/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
  })

  // State 3: Processing
  it('renders "Finalizing your recording..." in completing state', () => {
    mockStore.status = 'completing'
    renderPage()
    expect(screen.getByText('Finalizing your recording...')).toBeInTheDocument()
  })

  it('shows drain progress when outbox is draining', () => {
    mockStore.status = 'completing'
    renderPage()
    // The text "Processing audio..." appears when drainProgress is null
    expect(screen.getByText('Processing audio...')).toBeInTheDocument()
  })

  it('drain counter follows the live outbox size', async () => {
    mockStore.status = 'recording'
    mockStore.elapsed = 10
    mockStore.sessionId = 'sess-drain'
    mockStore.outboxSize = 4
    let releaseDrain: () => void = () => {}
    mockChunkUpload.waitForDrain.mockImplementation(
      () => new Promise<void>((resolve) => { releaseDrain = resolve }),
    )
    trackStatusOnce()

    const { router } = renderPage()
    fireEvent.click(screen.getByRole('button', { name: /stop/i }))

    await waitFor(() => {
      expect(screen.getByText('Uploading remaining audio... (0 of 4)')).toBeInTheDocument()
    })

    // Three chunks flushed: the counter must move, not sit at "0 of 4".
    // A same-route navigation re-renders the page without remounting it (and
    // must not trip the exit guard).
    mockStore.outboxSize = 1
    await act(async () => {
      await router.navigate('/record?tick=1')
    })
    expect(screen.getByText('Uploading remaining audio... (3 of 4)')).toBeInTheDocument()

    await act(async () => {
      releaseDrain()
    })
  })

  // Error state
  it('renders error message in error state', () => {
    mockStore.status = 'error'
    mockStore.errorMessage = 'Failed to start recording session.'
    renderPage()
    expect(screen.getByText('Recording Error')).toBeInTheDocument()
    expect(screen.getByText('Failed to start recording session.')).toBeInTheDocument()
  })

  it('renders "Try Again" button in error state', () => {
    mockStore.status = 'error'
    mockStore.errorMessage = 'Some error'
    renderPage()
    expect(screen.getByRole('button', { name: /try again/i })).toBeInTheDocument()
  })

  // Upload status indicator
  it('shows "Saving..." when outbox has unsent chunks', () => {
    mockStore.status = 'recording'
    mockStore.outboxSize = 3
    renderPage()
    expect(screen.getByText(/saving/i)).toBeInTheDocument()
  })

  it('shows "Saved" when outbox is empty and lastSavedAt is set', () => {
    mockStore.status = 'recording'
    mockStore.outboxSize = 0
    mockStore.lastSavedAt = Date.now()
    renderPage()
    expect(screen.getByText('Saved')).toBeInTheDocument()
  })

  it('awaits the chunk enqueue from onDataAvailable', async () => {
    renderPage()

    const chunk = new Blob(['audio'], { type: 'audio/webm' })
    await act(async () => {
      await capturedMediaRecorderOptions?.onDataAvailable?.(chunk, 7)
    })

    expect(mockChunkUpload.enqueue).toHaveBeenCalledWith(7, chunk)
  })

  it('stops the recording and warns when a chunk cannot be stored locally', async () => {
    mockStore.status = 'recording'
    mockStore.sessionId = 'sess-quota'
    mockChunkUpload.enqueue.mockRejectedValue(new Error('QuotaExceededError'))
    trackStatusOnce()

    renderPage()

    await act(async () => {
      await capturedMediaRecorderOptions?.onDataAvailable?.(new Blob(['audio']), 2)
    })

    expect(mockStore.setLocalWriteFailed).toHaveBeenCalledWith(true)
    expect(toast.error).toHaveBeenCalledWith(
      'This device could not store the latest audio. Recording stopped to protect what has already been captured.',
    )
    // Auto-stop: keep what was captured instead of guaranteeing a gap.
    await waitFor(() => {
      expect(mockMediaRecorder.stop).toHaveBeenCalledTimes(1)
    })
    // Stop must not complete silently — the tail may be missing.
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        'Some audio could not be stored on this device, so the end of this recording may be missing.',
      )
    })
  })

  it('prepares capture before creating server session and starting MediaRecorder', async () => {
    mockMediaRecorder.prepare.mockResolvedValue(undefined)
    mockCreateSession.mutateAsync.mockResolvedValue({
      id: 'sess-1',
      status: 'recording',
      mime_type: 'audio/webm',
      total_duration: 0,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    })
    mockMediaRecorder.start.mockResolvedValue(undefined)

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: /start recording/i }))

    await waitFor(() => {
      expect(mockMediaRecorder.prepare).toHaveBeenCalledTimes(1)
      expect(mockCreateSession.mutateAsync).toHaveBeenCalledTimes(1)
      expect(mockMediaRecorder.start).toHaveBeenCalledTimes(1)
    })

    expect(mockCreateSession.mutateAsync).toHaveBeenCalledWith(expect.objectContaining({
      capture_source: 'microphone',
    }))
    const prepareCallOrder = mockMediaRecorder.prepare.mock.invocationCallOrder[0]
    const createCallOrder = mockCreateSession.mutateAsync.mock.invocationCallOrder[0]
    const startCallOrder = mockMediaRecorder.start.mock.invocationCallOrder[0]
    expect(prepareCallOrder).toBeLessThan(createCallOrder)
    expect(createCallOrder).toBeLessThan(startCallOrder)
  })

  it('does not create a server session when mixed capture prepare fails', async () => {
    mockStore.captureSource = 'mixed_audio'
    mockMediaRecorder.prepare.mockRejectedValue(new Error('display audio unavailable'))

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: /start recording/i }))

    expect(await screen.findByText('Record meeting audio?')).toBeInTheDocument()
    expect(mockMediaRecorder.prepare).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'I have consent — choose audio' }))

    await waitFor(() => {
      expect(mockMediaRecorder.prepare).toHaveBeenCalledTimes(1)
    })
    expect(mockCreateSession.mutateAsync).not.toHaveBeenCalled()
    expect(mockMediaRecorder.start).not.toHaveBeenCalled()
  })

  it('revalidates a cached full quota before starting a recording', async () => {
    const fullUsage = {
      user_storage: {
        used_bytes: 100,
        limit_bytes: 100,
        remaining_bytes: 0,
        usage_percent: 100,
        warning_threshold_percent: 85,
        state: 'full' as const,
        enforced: true,
      },
    }
    mockUseUsageStats.mockReturnValue({ data: fullUsage, refetch: mockRefetchUsage })
    mockRefetchUsage.mockResolvedValue({ data: fullUsage })

    renderPage()

    expect(screen.getByRole('alert')).toHaveTextContent('Uploads and recordings are blocked')
    const start = screen.getByRole('button', { name: /start recording/i })
    expect(start).toBeEnabled()

    fireEvent.click(start)

    await waitFor(() => expect(mockRefetchUsage).toHaveBeenCalledTimes(1))
    expect(mockMediaRecorder.prepare).not.toHaveBeenCalled()
    expect(mockCreateSession.mutateAsync).not.toHaveBeenCalled()
  })

  it('permits display-only meeting capture when no microphone is available', async () => {
    mockStore.captureSource = 'mixed_audio'
    mockMicDevices.error = 'Microphone access denied.'
    mockMicDevices.devices = []
    mockCreateSession.mutateAsync.mockResolvedValue({ id: 'sess-display-only' })

    renderPage()
    const start = screen.getByRole('button', { name: /start recording/i })
    expect(start).toBeEnabled()
    fireEvent.click(start)
    fireEvent.click(await screen.findByRole('button', { name: 'I have consent — choose audio' }))

    await waitFor(() => {
      expect(mockCreateSession.mutateAsync).toHaveBeenCalledWith(expect.objectContaining({
        capture_source: 'mixed_audio',
      }))
      expect(mockMediaRecorder.start).toHaveBeenCalledTimes(1)
    })
  })

  it('does not create a server session if the prepared display track has ended', async () => {
    mockStore.captureSource = 'mixed_audio'
    mockMediaRecorder.isPreparedCaptureLive.mockReturnValue(false)

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: /start recording/i }))
    fireEvent.click(await screen.findByRole('button', { name: 'I have consent — choose audio' }))

    await waitFor(() => {
      expect(mockMediaRecorder.prepare).toHaveBeenCalledTimes(1)
    })
    expect(mockMediaRecorder.discardPrepared).toHaveBeenCalledTimes(1)
    expect(mockCreateSession.mutateAsync).not.toHaveBeenCalled()
    expect(mockMediaRecorder.start).not.toHaveBeenCalled()
  })

  it('releases the server session if display audio ends between session creation and recorder start', async () => {
    mockStore.captureSource = 'mixed_audio'
    mockCreateSession.mutateAsync.mockResolvedValue({ id: 'sess-display-ended-after-create' })
    mockMediaRecorder.isPreparedCaptureLive
      .mockReturnValueOnce(true)
      .mockReturnValueOnce(false)

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: /start recording/i }))
    fireEvent.click(await screen.findByRole('button', { name: 'I have consent — choose audio' }))

    await waitFor(() => {
      expect(mockCreateSession.mutateAsync).toHaveBeenCalledTimes(1)
      expect(mockReleaseSession.mutateAsync).toHaveBeenCalledWith('sess-display-ended-after-create')
      expect(mockAbandonSession.mutateAsync).toHaveBeenCalledWith('sess-display-ended-after-create')
    })
    expect(mockMediaRecorder.start).not.toHaveBeenCalled()
  })

  it('discards prepared local capture when session creation fails', async () => {
    mockMediaRecorder.prepare.mockResolvedValue(undefined)
    mockCreateSession.mutateAsync.mockRejectedValue(new Error('server down'))

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: /start recording/i }))

    await waitFor(() => {
      expect(mockMediaRecorder.discardPrepared).toHaveBeenCalledTimes(1)
    })
    expect(mockMediaRecorder.start).not.toHaveBeenCalled()
  })

  it('explains a 409 from session creation as an already-active session', async () => {
    mockCreateSession.mutateAsync.mockRejectedValue(new ApiError(409, 'Conflict'))

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: /start recording/i }))

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        'Another recording session is already active. Recover or discard it before starting a new one.',
      )
    })
    expect(mockStore.setError).toHaveBeenCalledWith(
      'Another recording session is already active. Recover or discard it before starting a new one.',
    )
  })

  it('navigates to library after stop completes', async () => {
    mockStore.status = 'recording'
    mockStore.elapsed = 10
    mockStore.sessionId = 'sess-processed-1'
    mockMediaRecorder.stop.mockResolvedValue(undefined)
    mockCompleteSession.mutateAsync.mockResolvedValue({ status: 'completing' })

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: /stop/i }))

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/library')
    })
  })

  it('recovers instead of stranding an orphaned session when completion conflicts', async () => {
    mockStore.status = 'recording'
    mockStore.elapsed = 10
    mockStore.sessionId = 'sess-orphaned-offline'
    mockCompleteSession.mutateAsync.mockRejectedValue(new ApiError(409, 'Conflict'))
    mockRecoverSession.mutateAsync.mockResolvedValue({ status: 'completing' })

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: /stop/i }))

    await waitFor(() => {
      expect(mockRecoverSession.mutateAsync).toHaveBeenCalledWith('sess-orphaned-offline')
      expect(mockNavigate).toHaveBeenCalledWith('/library')
    })
    expect(toast.info).toHaveBeenCalledWith(
      'Your connection interrupted the recording. The saved audio is processing now.',
    )
  })

  it('releases a native recorder failure into recovery only after capture stops', async () => {
    mockStore.status = 'recording'
    mockStore.sessionId = 'sess-native-error'

    renderPage()
    await act(async () => {
      capturedMediaRecorderOptions?.onCaptureInterrupted?.('Native recorder failed')
    })

    await waitFor(() => {
      expect(mockReleaseSession.mutateAsync).toHaveBeenCalledWith('sess-native-error')
      expect(mockRefetchInterrupted).toHaveBeenCalledTimes(1)
    })
    expect(mockMediaRecorder.stop).toHaveBeenCalledTimes(1)
    expect(toast.error).toHaveBeenCalledWith(
      'Native recorder failed Your saved audio is ready to recover.',
    )
  })


  it('completes the session only once when capture-ended fires twice in the same tick', async () => {
    mockStore.status = 'recording'
    mockStore.sessionId = 'sess-double-ended'
    // The backend rejects a second complete with 409 — simulate that so a
    // second concurrent run would surface an error toast.
    mockCompleteSession.mutateAsync
      .mockResolvedValueOnce({ status: 'completing' })
      .mockRejectedValueOnce(new Error('conflict'))

    renderPage()
    act(() => {
      // "Stop sharing" fires `ended` on multiple tracks in the same tick.
      capturedMediaRecorderOptions?.onCaptureEnded?.()
      capturedMediaRecorderOptions?.onCaptureEnded?.()
    })

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/library')
    })
    expect(mockCompleteSession.mutateAsync).toHaveBeenCalledTimes(1)
    expect(toast.error).not.toHaveBeenCalled()
    expect(mockStore.setError).not.toHaveBeenCalled()
  })

  it('completes the session only once when the stop button is double-clicked', async () => {
    mockStore.status = 'recording'
    mockStore.elapsed = 10
    mockStore.sessionId = 'sess-double-click'
    mockCompleteSession.mutateAsync
      .mockResolvedValueOnce({ status: 'completing' })
      .mockRejectedValueOnce(new Error('conflict'))

    renderPage()
    const stopButton = screen.getByRole('button', { name: /stop/i })
    fireEvent.click(stopButton)
    fireEvent.click(stopButton)

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/library')
    })
    expect(mockCompleteSession.mutateAsync).toHaveBeenCalledTimes(1)
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('completes the session when stop fires from a stale pre-recording closure', async () => {
    // In production the capture-ended and auto-stop listeners hold the
    // closure from the render in which prepare() ran — a snapshot where
    // status is still 'idle' and sessionId is null. handleStop must read
    // live store state, not that snapshot.
    mockStore.status = 'idle'
    renderPage()
    const idleRenderOptions = capturedMediaRecorderOptions

    // Recording then starts: live state moves on, the captured closure does not.
    mockStore.status = 'recording'
    mockStore.sessionId = 'sess-stale-closure'

    await act(async () => {
      idleRenderOptions?.onCaptureEnded?.()
    })

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/library')
    })
    expect(mockCompleteSession.mutateAsync).toHaveBeenCalledWith('sess-stale-closure')
    expect(mockMediaRecorder.stop).toHaveBeenCalledTimes(1)
  })

  it('ignores capture-ended stop requests when the session is already completing', async () => {
    mockStore.status = 'completing'
    mockStore.sessionId = 'sess-already-completing'

    renderPage()
    await act(async () => {
      capturedMediaRecorderOptions?.onCaptureEnded?.()
    })

    expect(mockMediaRecorder.stop).not.toHaveBeenCalled()
    expect(mockCompleteSession.mutateAsync).not.toHaveBeenCalled()
  })

  it('sends heartbeat on interval while recording', async () => {
    vi.useFakeTimers()
    mockStore.status = 'recording'
    mockStore.sessionId = 'sess-heartbeat-1'
    renderPage()

    await act(async () => {
      vi.advanceTimersByTime(46_000)
    })

    expect(mockSendHeartbeat).toHaveBeenCalledWith('sess-heartbeat-1')
    vi.useRealTimers()
  })

  it('sends heartbeat on interval while paused', async () => {
    vi.useFakeTimers()
    mockStore.status = 'paused'
    mockStore.sessionId = 'sess-heartbeat-2'
    renderPage()

    await act(async () => {
      vi.advanceTimersByTime(46_000)
    })

    expect(mockSendHeartbeat).toHaveBeenCalledWith('sess-heartbeat-2')
    vi.useRealTimers()
  })

  it('keeps sending heartbeats while the outbox drains', async () => {
    vi.useFakeTimers()
    // The server session stays `recording` until complete returns, and chunk
    // uploads only bump last_chunk_at — so a long drain still needs heartbeats.
    mockStore.status = 'completing'
    mockStore.sessionId = 'sess-heartbeat-drain'
    renderPage()

    await act(async () => {
      vi.advanceTimersByTime(46_000)
    })

    expect(mockSendHeartbeat).toHaveBeenCalledWith('sess-heartbeat-drain')
    vi.useRealTimers()
  })

  it('heartbeat cadence safety helper tolerates timer clamping assumptions', () => {
    expect(isHeartbeatCadenceSafe(45_000, 30)).toBe(true)
    expect(isHeartbeatCadenceSafe(700_000, 30)).toBe(false)
  })

  it('navigates to library after recovering interrupted session', async () => {
    mockInterruptedData.data = {
      items: [{
        id: 'sess-recover-1',
        status: 'interrupted',
        mime_type: 'audio/webm',
        total_duration: 0,
        created_at: '2026-02-28T10:00:00Z',
        updated_at: '2026-02-28T10:05:00Z',
      }],
    }
    mockRecoverSession.mutateAsync.mockResolvedValue({ status: 'completing' })

    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: 'Recover' }))

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/library')
    })
  })

  it('clears encrypted local chunks and their session key when an interrupted recording is discarded', async () => {
    mockInterruptedData.data = {
      items: [{
        id: 'sess-discard-local',
        status: 'interrupted',
        mime_type: 'audio/webm',
        total_duration: 0,
        created_at: '2026-02-28T10:00:00Z',
        updated_at: '2026-02-28T10:05:00Z',
      }],
    }
    const { deleteBySession } = await import('@/lib/chunk-outbox')

    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: 'Discard' }))

    await waitFor(() => {
      expect(mockAbandonSession.mutateAsync).toHaveBeenCalledWith('sess-discard-local')
      expect(deleteBySession).toHaveBeenCalledWith('sess-discard-local')
    })
  })
})

describe('RecordingPage recovery with buffered chunks', () => {
  const interruptedSession = {
    id: 'sess-recover-outbox',
    status: 'interrupted',
    mime_type: 'audio/webm',
    total_duration: 0,
    created_at: '2026-02-28T10:00:00Z',
    updated_at: '2026-02-28T10:05:00Z',
  }

  beforeEach(async () => {
    vi.clearAllMocks()
    mockNavigate.mockReset()
    mockStore.status = 'idle'
    mockStore.sessionId = null
    mockStore.localWriteFailed = false
    mockStore.flushFailure = null
    mockInterruptedData.data = { items: [interruptedSession] }
    mockFetchActiveSession.mockResolvedValue(null)
    mockStore.setStatus.mockReset()
    mockRecoverSession.mutateAsync.mockReset()
    mockRecoverSession.mutateAsync.mockResolvedValue({ status: 'completing' })
    mockUploadChunk.mockReset()
    mockUploadChunk.mockResolvedValue(undefined)

    const { getAllBySession, deleteChunk } = await import('@/lib/chunk-outbox')
    // Deliberately out of seq order: recovery must sort before uploading.
    vi.mocked(getAllBySession).mockResolvedValue([
      { id: 22, sessionId: interruptedSession.id, seq: 2, blob: new Blob(['b']), createdAt: 2 },
      { id: 11, sessionId: interruptedSession.id, seq: 1, blob: new Blob(['a']), createdAt: 1 },
    ])
    vi.mocked(deleteChunk).mockResolvedValue(undefined)
  })

  afterEach(async () => {
    const { getAllBySession } = await import('@/lib/chunk-outbox')
    vi.mocked(getAllBySession).mockResolvedValue([])
    vi.restoreAllMocks()
  })

  it('uploads buffered chunks in seq order before recovering, deleting each after success', async () => {
    const { deleteChunk } = await import('@/lib/chunk-outbox')

    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: 'Recover' }))

    await waitFor(() => {
      expect(mockRecoverSession.mutateAsync).toHaveBeenCalledWith(interruptedSession.id)
    })

    expect(mockUploadChunk.mock.calls.map((call) => call[1])).toEqual([1, 2])
    expect(vi.mocked(deleteChunk).mock.calls.map((call) => call[0])).toEqual([11, 22])
    expect(
      mockUploadChunk.mock.invocationCallOrder[1],
    ).toBeLessThan(mockRecoverSession.mutateAsync.mock.invocationCallOrder[0])
    expect(mockNavigate).toHaveBeenCalledWith('/library')
  })

  it('recovers with server-side audio when a buffered chunk is permanently rejected', async () => {
    const { deleteChunk } = await import('@/lib/chunk-outbox')
    // Seq 1 was lost locally, so seq 2 lands on a gap the server rejects.
    mockUploadChunk.mockRejectedValueOnce(new ApiError(409, 'Conflict'))

    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: 'Recover' }))

    await waitFor(() => {
      expect(mockRecoverSession.mutateAsync).toHaveBeenCalledWith(interruptedSession.id)
    })

    expect(toast.error).toHaveBeenCalledWith(
      '1 locally saved segment(s) could not be attached. Recovering with the audio the server already has.',
    )
    // Only the chunk that uploaded cleanly is dropped locally.
    expect(vi.mocked(deleteChunk).mock.calls.map((call) => call[0])).toEqual([22])
    expect(mockNavigate).toHaveBeenCalledWith('/library')
  })

  it('aborts recovery when a buffered chunk fails transiently', async () => {
    mockUploadChunk.mockRejectedValue(new ApiError(0, 'Network error'))

    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: 'Recover' }))

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith('Failed to recover recording.')
    })
    expect(mockRecoverSession.mutateAsync).not.toHaveBeenCalled()
  })
})

describe('RecordingPage exit guard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockNavigate.mockReset()
    mockStore.status = 'recording'
    mockStore.sessionId = 'sess-guard'
    mockStore.outboxSize = 0
    mockStore.localWriteFailed = false
    mockStore.flushFailure = null
    mockInterruptedData.data = { items: [] }
    mockFetchActiveSession.mockResolvedValue(null)
    mockStore.setStatus.mockReset()
    mockMediaRecorder.stop.mockReset()
    mockMediaRecorder.stop.mockResolvedValue(undefined)
    mockChunkUpload.waitForDrain.mockReset()
    mockChunkUpload.waitForDrain.mockResolvedValue(undefined)
    mockChunkUpload.clearSession.mockReset()
    mockChunkUpload.clearSession.mockResolvedValue(undefined)
    mockCompleteSession.mutateAsync.mockReset()
    mockCompleteSession.mutateAsync.mockResolvedValue({ status: 'completing' })
    mockReleaseSession.mutateAsync.mockReset()
    mockReleaseSession.mutateAsync.mockResolvedValue({ status: 'interrupted' })
    mockAbandonSession.mutateAsync.mockReset()
    mockAbandonSession.mutateAsync.mockResolvedValue(undefined)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('blocks in-app navigation while recording', async () => {
    const { router } = renderPage()

    await act(async () => {
      await router.navigate('/media')
    })

    expect(screen.getByText('Recording in progress')).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/record')
    expect(screen.queryByText('Media Library')).not.toBeInTheDocument()
  })

  it('keeps recording when the guard is dismissed', async () => {
    const { router } = renderPage()
    await act(async () => {
      await router.navigate('/media')
    })

    fireEvent.click(screen.getByRole('button', { name: 'Keep recording' }))

    await waitFor(() => {
      expect(screen.queryByText('Recording in progress')).not.toBeInTheDocument()
    })
    expect(router.state.location.pathname).toBe('/record')
    expect(mockMediaRecorder.stop).not.toHaveBeenCalled()
  })

  it('runs the normal stop flow when the user chooses stop & save', async () => {
    const { router } = renderPage()
    await act(async () => {
      await router.navigate('/media')
    })

    fireEvent.click(screen.getByRole('button', { name: 'Stop & save' }))

    await waitFor(() => {
      expect(mockCompleteSession.mutateAsync).toHaveBeenCalledWith('sess-guard')
    })
    expect(mockMediaRecorder.stop).toHaveBeenCalledTimes(1)
    expect(mockNavigate).toHaveBeenCalledWith('/library')
  })

  it('releases, abandons and clears the outbox when the user discards', async () => {
    const { router } = renderPage()
    await act(async () => {
      await router.navigate('/media')
    })

    fireEvent.click(screen.getByRole('button', { name: 'Discard recording' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Discard permanently' }))

    await waitFor(() => {
      expect(mockAbandonSession.mutateAsync).toHaveBeenCalledWith('sess-guard')
    })
    expect(mockMediaRecorder.stop).toHaveBeenCalledTimes(1)
    expect(mockReleaseSession.mutateAsync).toHaveBeenCalledWith('sess-guard')
    // Release must precede abandon: the session is still `recording`.
    expect(mockReleaseSession.mutateAsync.mock.invocationCallOrder[0]).toBeLessThan(
      mockAbandonSession.mutateAsync.mock.invocationCallOrder[0],
    )
    expect(mockChunkUpload.clearSession).toHaveBeenCalled()
    expect(mockCompleteSession.mutateAsync).not.toHaveBeenCalled()
    expect(mockNavigate).toHaveBeenCalledWith('/library')
  })

  it('does not block navigation once the recording is over', () => {
    const from = loc('/record')
    const to = loc('/media')
    mockStore.status = 'recording'
    expect(shouldBlockRecordingExit({ currentLocation: from, nextLocation: to })).toBe(true)
    mockStore.status = 'completing'
    expect(shouldBlockRecordingExit({ currentLocation: from, nextLocation: to })).toBe(true)
    mockStore.status = 'idle'
    expect(shouldBlockRecordingExit({ currentLocation: from, nextLocation: to })).toBe(false)
    mockStore.status = 'error'
    expect(shouldBlockRecordingExit({ currentLocation: from, nextLocation: to })).toBe(false)
    // Same-route navigations (e.g. query-param updates) never block.
    mockStore.status = 'recording'
    expect(shouldBlockRecordingExit({ currentLocation: from, nextLocation: loc('/record') })).toBe(
      false,
    )
  })
})

describe('RecordingPage stuck upload recovery', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockNavigate.mockReset()
    mockStore.status = 'completing'
    mockStore.sessionId = 'sess-stuck'
    mockStore.outboxSize = 3
    mockStore.localWriteFailed = false
    mockStore.flushFailure = { status: 413, seq: 5 }
    mockInterruptedData.data = { items: [] }
    mockFetchActiveSession.mockResolvedValue(null)
    mockStore.setStatus.mockReset()
    mockMediaRecorder.stop.mockReset()
    mockMediaRecorder.stop.mockResolvedValue(undefined)
    mockChunkUpload.retryFlush.mockReset()
    mockChunkUpload.retryFlush.mockResolvedValue(undefined)
    mockChunkUpload.clearSession.mockReset()
    mockChunkUpload.clearSession.mockResolvedValue(undefined)
    mockCompleteSession.mutateAsync.mockReset()
    mockCompleteSession.mutateAsync.mockResolvedValue({ status: 'completing' })
    mockReleaseSession.mutateAsync.mockReset()
    mockReleaseSession.mutateAsync.mockResolvedValue({ status: 'interrupted' })
    mockAbandonSession.mutateAsync.mockReset()
    mockAbandonSession.mutateAsync.mockResolvedValue(undefined)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('replaces the endless spinner with retry and give-up choices', () => {
    renderPage()

    expect(screen.getByText('Upload failed')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry upload' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: "Save what's uploaded" })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Discard recording' })).toBeInTheDocument()
  })

  it('keeps retry available and opens Library separately after a storage quota rejection', async () => {
    mockStore.flushFailure = { status: 409, seq: 5, code: 'storage_quota_exceeded' }
    renderPage()

    expect(screen.getByText('Storage limit reached. Uploads and recordings are blocked. Free audio storage in your Library.')).toBeInTheDocument()
    const retry = screen.getByRole('button', { name: 'Retry upload' })
    const library = screen.getByRole('link', { name: 'Manage storage in Library' })
    expect(library).toHaveAttribute('href', '/library')
    expect(library).toHaveAttribute('target', '_blank')
    expect(library).toHaveAttribute('rel', 'noopener noreferrer')
    expect(screen.getByRole('button', { name: "Save what's uploaded" })).toBeInTheDocument()

    fireEvent.click(retry)
    await waitFor(() => expect(mockChunkUpload.retryFlush).toHaveBeenCalledTimes(1))
  })

  it('retries the flush and completes the session when the retry succeeds', async () => {
    mockChunkUpload.retryFlush.mockImplementation(async () => {
      mockStore.flushFailure = null
    })

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'Retry upload' }))

    await waitFor(() => {
      expect(mockCompleteSession.mutateAsync).toHaveBeenCalledWith('sess-stuck')
    })
    expect(mockChunkUpload.retryFlush).toHaveBeenCalledTimes(1)
  })

  it('keeps the failure panel up when the retry is rejected again', async () => {
    mockChunkUpload.retryFlush.mockRejectedValue(new Error('chunk 5 rejected with status 413'))

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'Retry upload' }))

    await waitFor(() => {
      expect(mockChunkUpload.retryFlush).toHaveBeenCalledTimes(1)
    })
    expect(mockCompleteSession.mutateAsync).not.toHaveBeenCalled()
    expect(screen.getByText('Upload failed')).toBeInTheDocument()
  })

  it('completes with the uploaded audio after the data-loss warning is confirmed', async () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: "Save what's uploaded" }))

    expect(
      await screen.findByText('Save only the uploaded audio?'),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Save partial recording' }))

    await waitFor(() => {
      expect(mockCompleteSession.mutateAsync).toHaveBeenCalledWith('sess-stuck')
    })
    expect(mockChunkUpload.clearSession).toHaveBeenCalled()
  })
})

describe('RecordingPage active session takeover', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockNavigate.mockReset()
    mockStore.status = 'idle'
    mockStore.sessionId = null
    mockStore.flushFailure = null
    mockStore.localWriteFailed = false
    mockInterruptedData.data = { items: [] }
    mockStore.setStatus.mockReset()
    mockFetchActiveSession.mockReset()
    mockReleaseSession.mutateAsync.mockReset()
    mockReleaseSession.mutateAsync.mockResolvedValue({ status: 'interrupted' })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  const activeSession = {
    id: 'sess-orphan',
    status: 'recording',
    mime_type: 'audio/webm',
    total_duration: 0,
    chunk_count: 4,
    created_at: '2026-02-28T10:00:00Z',
    updated_at: '2026-02-28T10:05:00Z',
  }

  // A fresh mount is not proof the session is stale: it may be a healthy
  // recording in another tab that must not be interrupted behind the user's back.
  it('asks before touching a session the server still reports as active', async () => {
    mockFetchActiveSession.mockResolvedValue(activeSession)

    renderPage()

    expect(await screen.findByText('Active recording found')).toBeInTheDocument()
    expect(screen.getByText('4 segment(s) uploaded so far')).toBeInTheDocument()
    expect(mockReleaseSession.mutateAsync).not.toHaveBeenCalled()
    // Starting a new recording is off the table until the user decides. The
    // modal dialog aria-hides the page behind it, so query it explicitly.
    expect(
      screen.getByRole('button', { name: /start recording/i, hidden: true }),
    ).toBeDisabled()
  })

  it('releases the session only after the user takes over', async () => {
    mockFetchActiveSession.mockResolvedValue(activeSession)

    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: 'Take over' }))

    await waitFor(() => {
      expect(mockReleaseSession.mutateAsync).toHaveBeenCalledWith('sess-orphan')
    })
    await waitFor(() => {
      expect(screen.queryByText('Active recording found')).not.toBeInTheDocument()
    })
    expect(toast.info).toHaveBeenCalledWith(
      'A recording session was left open. You can recover or discard it now.',
    )
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('leaves the session alone and navigates away when the user declines', async () => {
    mockFetchActiveSession.mockResolvedValue(activeSession)

    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: 'Go to library' }))

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/library')
    })
    expect(mockReleaseSession.mutateAsync).not.toHaveBeenCalled()
  })

  it('keeps the recorder usable when the active-session check fails', async () => {
    mockFetchActiveSession.mockRejectedValue(new ApiError(500, 'Server error'))

    renderPage()

    await waitFor(() => {
      expect(mockFetchActiveSession).toHaveBeenCalledTimes(1)
    })
    expect(screen.queryByText('Active recording found')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /start recording/i })).toBeEnabled()
  })

  it('offers Recover/Discard for the released session', async () => {
    mockFetchActiveSession.mockResolvedValue(null)
    mockInterruptedData.data = {
      items: [{
        id: 'sess-orphan',
        status: 'interrupted',
        mime_type: 'audio/webm',
        total_duration: 0,
        created_at: '2026-02-28T10:00:00Z',
        updated_at: '2026-02-28T10:05:00Z',
      }],
    }

    renderPage()

    expect(await screen.findByRole('button', { name: 'Recover' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Discard' })).toBeInTheDocument()
  })

  it('does nothing when the server reports no active session', async () => {
    mockFetchActiveSession.mockResolvedValue(null)

    renderPage()

    await waitFor(() => {
      expect(mockFetchActiveSession).toHaveBeenCalledTimes(1)
    })
    expect(screen.queryByText('Active recording found')).not.toBeInTheDocument()
    expect(mockReleaseSession.mutateAsync).not.toHaveBeenCalled()
  })

  // StrictMode double-invokes mount effects; the ref guard must keep the
  // check (and therefore any release) to exactly one run.
  it('checks the active session only once per mount', async () => {
    mockFetchActiveSession.mockResolvedValue(activeSession)

    const { rerenderPage } = renderPage()
    await screen.findByText('Active recording found')
    rerenderPage()

    await waitFor(() => {
      expect(mockFetchActiveSession).toHaveBeenCalledTimes(1)
    })
  })
})
