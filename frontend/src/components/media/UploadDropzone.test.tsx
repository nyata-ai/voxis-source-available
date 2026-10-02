import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { useUploadStore } from '@/stores/upload'
import { useCaptureStore } from '@/stores/capture'
import { ApiError } from '@/lib/api-client'
import { resetUploadSchedulerForTests } from './upload-utils'
import { UploadDropzone } from './UploadDropzone'

const { mockMutateAsync } = vi.hoisted(() => ({ mockMutateAsync: vi.fn() }))
vi.mock('@/hooks/useMedia', () => ({
  useUploadMedia: () => ({
    mutateAsync: mockMutateAsync,
    isPending: false,
  }),
}))

const { mockUseUsageStats } = vi.hoisted(() => ({ mockUseUsageStats: vi.fn() }))
vi.mock('@/hooks/useUsageStats', () => ({
  useUsageStats: mockUseUsageStats,
}))

// The dropzone links completed rows to /media/:id and navigates there itself
// for single-file uploads, so tests render it inside a router with a stub
// media page to land on.
function renderDropzone() {
  return render(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route path="/" element={<UploadDropzone />} />
        <Route path="/media/:id" element={<div data-testid="media-page" />} />
      </Routes>
    </MemoryRouter>
  )
}

function dropEvent(files: File[]) {
  return {
    dataTransfer: {
      files,
      items: files.map((file) => ({
        kind: 'file',
        type: file.type,
        getAsFile: () => file,
      })),
      types: ['Files'],
    },
  }
}

describe('UploadDropzone', () => {
  beforeEach(() => {
    useUploadStore.getState().reset()
    resetUploadSchedulerForTests()
    useCaptureStore.setState({ open: false, tab: 'upload' })
    mockMutateAsync.mockReset()
    mockUseUsageStats.mockReturnValue({
      data: undefined,
      refetch: vi.fn().mockResolvedValue({ data: undefined }),
    })
  })

  it('renders drop area with instructions', () => {
    renderDropzone()

    expect(
      screen.getByText(/drag & drop audio files here/i)
    ).toBeInTheDocument()
  })

  it('shows accepted file formats', () => {
    renderDropzone()

    expect(
      screen.getByText(/MP3, WAV, OGG, FLAC, M4A, WebM, AAC, Opus/i)
    ).toBeInTheDocument()
  })

  it('has accessible upload button', () => {
    renderDropzone()

    expect(
      screen.getByRole('button', { name: /browse/i })
    ).toBeInTheDocument()
  })

  it('shows file size limit', () => {
    renderDropzone()

    expect(screen.getByText(/1.*GB/i)).toBeInTheDocument()
  })

  it('keeps the quota warning visible while uploads are available', () => {
    mockUseUsageStats.mockReturnValue({
      data: {
        user_storage: {
          used_bytes: 85,
          limit_bytes: 100,
          remaining_bytes: 15,
          usage_percent: 85,
          warning_threshold_percent: 85,
          state: 'warning',
          enforced: true,
        },
      },
      refetch: vi.fn().mockResolvedValue({ data: undefined }),
    })

    renderDropzone()

    expect(screen.getByRole('status')).toHaveTextContent('Storage is almost full: 85 B of 100 B used, 15 B remaining.')
  })

  it('displays queued files', () => {
    const file = new File(['audio-data'], 'recording.mp3', {
      type: 'audio/mpeg',
    })
    useUploadStore.getState().addFiles([file])

    renderDropzone()

    expect(screen.getByText('recording.mp3')).toBeInTheDocument()
  })

  it('shows upload progress', () => {
    const file = new File(['audio-data'], 'recording.mp3', {
      type: 'audio/mpeg',
    })
    useUploadStore.getState().addFiles([file])
    const item = useUploadStore.getState().queue[0]
    useUploadStore.getState().setProgress(item.id, 45)

    renderDropzone()

    const progressBar = screen.getByRole('progressbar')
    expect(progressBar).toBeInTheDocument()
    expect(progressBar).toHaveAttribute('aria-valuenow', '45')
  })

  it('shows error state', () => {
    const file = new File(['audio-data'], 'recording.mp3', {
      type: 'audio/mpeg',
    })
    useUploadStore.getState().addFiles([file])
    const item = useUploadStore.getState().queue[0]
    useUploadStore.getState().setError(item.id, 'Upload failed: server error')

    renderDropzone()

    expect(
      screen.getByText('Upload failed: server error')
    ).toBeInTheDocument()
  })

  it('allows removing items from queue', async () => {
    const user = userEvent.setup()
    const file = new File(['audio-data'], 'recording.mp3', {
      type: 'audio/mpeg',
    })
    useUploadStore.getState().addFiles([file])

    renderDropzone()

    const removeButton = screen.getByRole('button', { name: /remove/i })
    await user.click(removeButton)

    expect(useUploadStore.getState().queue).toHaveLength(0)
  })

  it('shows processing state with spinner text', () => {
    const file = new File(['audio-data'], 'recording.mp3', {
      type: 'audio/mpeg',
    })
    useUploadStore.getState().addFiles([file])
    const item = useUploadStore.getState().queue[0]
    useUploadStore.getState().setProcessing(item.id)

    renderDropzone()

    expect(
      screen.getByText('Processing on server...')
    ).toBeInTheDocument()
  })

  it('shows rejected files with reasons', () => {
    const file = new File(['text'], 'notes.txt', { type: 'text/plain' })
    useUploadStore.getState().addFiles([file])

    renderDropzone()

    expect(screen.getByText('notes.txt')).toBeInTheDocument()
    expect(screen.getByText(/unsupported file type/i)).toBeInTheDocument()
  })

  it('links a completed row to its media page', () => {
    const file = new File(['audio-data'], 'recording.mp3', {
      type: 'audio/mpeg',
    })
    useUploadStore.getState().addFiles([file])
    const item = useUploadStore.getState().queue[0]
    useUploadStore.getState().setComplete(item.id, 'media-42')

    renderDropzone()

    const link = screen.getByRole('link', { name: 'View file: recording.mp3' })
    expect(link).toHaveAttribute('href', '/media/media-42')
  })

  it('closes the capture dialog and clears the settled queue when a view link is followed', async () => {
    const user = userEvent.setup()
    const file = new File(['audio-data'], 'recording.mp3', {
      type: 'audio/mpeg',
    })
    useUploadStore.getState().addFiles([file])
    const item = useUploadStore.getState().queue[0]
    useUploadStore.getState().setComplete(item.id, 'media-42')
    useCaptureStore.getState().openCapture('upload')

    renderDropzone()
    await user.click(screen.getByRole('link', { name: 'View file: recording.mp3' }))

    expect(useCaptureStore.getState().open).toBe(false)
    expect(useUploadStore.getState().queue).toHaveLength(0)
    expect(screen.getByTestId('media-page')).toBeInTheDocument()
  })

  it('keeps in-flight rows when a view link is followed mid-batch', async () => {
    const user = userEvent.setup()
    useUploadStore.getState().addFiles([
      new File(['a'], 'done.mp3', { type: 'audio/mpeg' }),
      new File(['b'], 'busy.mp3', { type: 'audio/mpeg' }),
    ])
    const [done, busy] = useUploadStore.getState().queue
    useUploadStore.getState().setComplete(done.id, 'media-1')
    useUploadStore.getState().setProgress(busy.id, 40)
    useCaptureStore.getState().openCapture('upload')

    renderDropzone()
    await user.click(screen.getByRole('link', { name: 'View file: done.mp3' }))

    expect(useCaptureStore.getState().open).toBe(false)
    expect(useUploadStore.getState().queue).toHaveLength(2)
  })

  it('navigates to the media page when a lone upload completes in the open dialog', async () => {
    mockMutateAsync.mockResolvedValue({ id: 'media-99' })
    useCaptureStore.getState().openCapture('upload')

    renderDropzone()

    const dropzone = screen.getByText(/drag & drop audio files here/i).parentElement!
    const file = new File(['audio-data'], 'recording.mp3', { type: 'audio/mpeg' })
    fireEvent.drop(dropzone, dropEvent([file]))

    await waitFor(() => {
      expect(screen.getByTestId('media-page')).toBeInTheDocument()
    })
    expect(useCaptureStore.getState().open).toBe(false)
    expect(useUploadStore.getState().queue).toHaveLength(0)
  })

  it('waits and retries when the server is out of upload capacity', async () => {
    const full = new ApiError(503, 'Service Unavailable', { error: 'temporarily_unavailable' })
    full.retryAfter = 1
    mockMutateAsync.mockRejectedValueOnce(full).mockResolvedValueOnce({ id: 'media-7' })

    renderDropzone()

    const dropzone = screen.getByText(/drag & drop audio files here/i).parentElement!
    fireEvent.drop(dropzone, dropEvent([new File(['a'], 'a.mp3', { type: 'audio/mpeg' })]))

    // First answer: server full → the row goes back to the queue, not to error.
    await waitFor(() => {
      expect(useUploadStore.getState().queue[0].status).toBe('queued')
    })
    // After Retry-After the upload is sent again and completes.
    await waitFor(
      () => {
        expect(useUploadStore.getState().queue[0].status).toBe('complete')
      },
      { timeout: 4000 }
    )
    expect(mockMutateAsync).toHaveBeenCalledTimes(2)
    expect(useUploadStore.getState().queue[0].mediaId).toBe('media-7')
  })

  it('stops retrying when the parked row is removed', async () => {
    const full = new ApiError(503, 'Service Unavailable', { error: 'temporarily_unavailable' })
    full.retryAfter = 1
    mockMutateAsync.mockRejectedValue(full)

    renderDropzone()

    const dropzone = screen.getByText(/drag & drop audio files here/i).parentElement!
    fireEvent.drop(dropzone, dropEvent([new File(['a'], 'a.mp3', { type: 'audio/mpeg' })]))
    await waitFor(() => {
      expect(useUploadStore.getState().queue[0].status).toBe('queued')
    })
    useUploadStore.getState().reset()

    // Past the retry delay the file must NOT be sent again for a row nobody can see.
    await new Promise((resolve) => setTimeout(resolve, 1600))
    expect(mockMutateAsync).toHaveBeenCalledTimes(1)
    expect(useUploadStore.getState().queue).toHaveLength(0)
  })

  it('sends at most two files at once and starts the rest as uploads settle', async () => {
    const pending: Array<(value: { id: string }) => void> = []
    mockMutateAsync.mockImplementation(
      () => new Promise<{ id: string }>((resolve) => pending.push(resolve))
    )

    renderDropzone()

    const dropzone = screen.getByText(/drag & drop audio files here/i).parentElement!
    fireEvent.drop(
      dropzone,
      dropEvent([
        new File(['a'], 'a.mp3', { type: 'audio/mpeg' }),
        new File(['b'], 'b.mp3', { type: 'audio/mpeg' }),
        new File(['c'], 'c.mp3', { type: 'audio/mpeg' }),
      ])
    )

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledTimes(2)
    })
    expect(useUploadStore.getState().queue.map((q) => q.status)).toEqual([
      'uploading',
      'uploading',
      'queued',
    ])

    pending[0]({ id: 'media-a' })
    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledTimes(3)
    })
    expect(useUploadStore.getState().queue.map((q) => q.status)).toEqual([
      'complete',
      'uploading',
      'uploading',
    ])

    pending[1]({ id: 'media-b' })
    pending[2]({ id: 'media-c' })
    await waitFor(() => {
      expect(useUploadStore.getState().queue.every((q) => q.status === 'complete')).toBe(true)
    })
  })

  it('does not retry a 503 that is not a capacity refusal', async () => {
    mockMutateAsync.mockRejectedValue(new ApiError(503, 'Service Unavailable', { error: 'boom' }))

    renderDropzone()

    const dropzone = screen.getByText(/drag & drop audio files here/i).parentElement!
    fireEvent.drop(dropzone, dropEvent([new File(['a'], 'a.mp3', { type: 'audio/mpeg' })]))

    await waitFor(() => {
      expect(useUploadStore.getState().queue[0].status).toBe('error')
    })
    expect(mockMutateAsync).toHaveBeenCalledTimes(1)
  })

  it('does not auto-navigate for a multi-file batch', async () => {
    mockMutateAsync
      .mockResolvedValueOnce({ id: 'media-1' })
      .mockResolvedValueOnce({ id: 'media-2' })
    useCaptureStore.getState().openCapture('upload')

    renderDropzone()

    const dropzone = screen.getByText(/drag & drop audio files here/i).parentElement!
    fireEvent.drop(
      dropzone,
      dropEvent([
        new File(['a'], 'one.mp3', { type: 'audio/mpeg' }),
        new File(['b'], 'two.mp3', { type: 'audio/mpeg' }),
      ])
    )

    await waitFor(() => {
      expect(screen.getByRole('link', { name: 'View file: one.mp3' })).toBeInTheDocument()
      expect(screen.getByRole('link', { name: 'View file: two.mp3' })).toBeInTheDocument()
    })
    expect(screen.queryByTestId('media-page')).not.toBeInTheDocument()
    expect(useCaptureStore.getState().open).toBe(true)
  })

  it('keeps the dialog and queue when a view link is opened with a modifier key', () => {
    // Ctrl/Cmd-click opens the target in a new tab; the current page never
    // navigates, so the dialog and the other rows' links must survive.
    useUploadStore.getState().addFiles([
      new File(['a'], 'one.mp3', { type: 'audio/mpeg' }),
      new File(['b'], 'two.mp3', { type: 'audio/mpeg' }),
    ])
    const [one, two] = useUploadStore.getState().queue
    useUploadStore.getState().setComplete(one.id, 'media-1')
    useUploadStore.getState().setComplete(two.id, 'media-2')
    useCaptureStore.getState().openCapture('upload')

    renderDropzone()
    fireEvent.click(screen.getByRole('link', { name: 'View file: one.mp3' }), {
      ctrlKey: true,
    })

    expect(useCaptureStore.getState().open).toBe(true)
    expect(useUploadStore.getState().queue).toHaveLength(2)
  })

  it('does not auto-navigate while a second drop is still being admitted', async () => {
    // The lone upload completes while another drop is awaiting the quota
    // refetch (so the queue does not yet contain the new file). Navigating on
    // the stale count would close the dialog under the incoming batch.
    let resolveUpload!: (value: { id: string }) => void
    mockMutateAsync.mockImplementationOnce(
      () => new Promise((resolve) => { resolveUpload = resolve })
    )
    const firstRefetch = vi.fn().mockResolvedValue({ data: undefined })
    const hangingRefetch = vi.fn().mockReturnValue(new Promise(() => {}))
    mockUseUsageStats.mockReturnValue({
      data: undefined,
      refetch: (...args: unknown[]) =>
        firstRefetch.mock.calls.length === 0
          ? firstRefetch(...args)
          : hangingRefetch(...args),
    })
    useCaptureStore.getState().openCapture('upload')

    renderDropzone()

    const dropzone = screen.getByText(/drag & drop audio files here/i).parentElement!
    fireEvent.drop(dropzone, dropEvent([new File(['a'], 'first.mp3', { type: 'audio/mpeg' })]))
    await waitFor(() => {
      expect(useUploadStore.getState().queue).toHaveLength(1)
    })

    // Second drop enters onDrop and stalls on the quota refetch.
    fireEvent.drop(dropzone, dropEvent([new File(['b'], 'second.mp3', { type: 'audio/mpeg' })]))
    await waitFor(() => {
      expect(hangingRefetch).toHaveBeenCalled()
    })

    resolveUpload({ id: 'media-1' })
    await waitFor(() => {
      expect(screen.getByRole('link', { name: 'View file: first.mp3' })).toBeInTheDocument()
    })
    expect(screen.queryByTestId('media-page')).not.toBeInTheDocument()
    expect(useCaptureStore.getState().open).toBe(true)
  })

  it('does not auto-navigate while a rejection notice is showing', async () => {
    // A drop can admit one file and reject a sibling (here: storage quota).
    // Navigating away would hide the rejection notice before it can be read.
    mockMutateAsync.mockResolvedValue({ id: 'media-1' })
    mockUseUsageStats.mockReturnValue({
      data: {
        user_storage: {
          used_bytes: 95,
          limit_bytes: 100,
          remaining_bytes: 5,
          usage_percent: 95,
          warning_threshold_percent: 85,
          state: 'warning',
          enforced: true,
        },
      },
      refetch: vi.fn().mockResolvedValue({ data: undefined }),
    })
    useCaptureStore.getState().openCapture('upload')

    renderDropzone()

    const dropzone = screen.getByText(/drag & drop audio files here/i).parentElement!
    fireEvent.drop(
      dropzone,
      dropEvent([
        new File(['a'], 'fits.mp3', { type: 'audio/mpeg' }),
        new File(['x'.repeat(50)], 'too-big.mp3', { type: 'audio/mpeg' }),
      ])
    )

    await waitFor(() => {
      expect(screen.getByRole('link', { name: 'View file: fits.mp3' })).toBeInTheDocument()
    })
    expect(screen.getByText('too-big.mp3')).toBeInTheDocument()
    expect(screen.queryByTestId('media-page')).not.toBeInTheDocument()
    expect(useCaptureStore.getState().open).toBe(true)
  })

  it('does not auto-navigate when the upload outlives the dialog', async () => {
    mockMutateAsync.mockResolvedValue({ id: 'media-77' })
    useCaptureStore.setState({ open: false, tab: 'upload' })

    renderDropzone()

    const dropzone = screen.getByText(/drag & drop audio files here/i).parentElement!
    fireEvent.drop(
      dropzone,
      dropEvent([new File(['a'], 'late.mp3', { type: 'audio/mpeg' })])
    )

    await waitFor(() => {
      expect(screen.getByRole('link', { name: 'View file: late.mp3' })).toBeInTheDocument()
    })
    expect(screen.queryByTestId('media-page')).not.toBeInTheDocument()
  })
})
