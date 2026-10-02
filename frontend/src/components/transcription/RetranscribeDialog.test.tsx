import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RetranscribeDialog } from './RetranscribeDialog'
import type { TranscriptionItem } from '@/types/transcription'

const mockNavigate = vi.fn()
const mockInvalidateQueries = vi.fn()

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
}))

const mockDelete = vi.fn()
const mockPost = vi.fn()

vi.mock('@/lib/api-client', () => ({
  apiClient: {
    delete: (...args: unknown[]) => mockDelete(...args),
    post: (...args: unknown[]) => mockPost(...args),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { toast } from '@/lib/toast'

const mockTranscription: TranscriptionItem = {
  id: 'tx-1',
  organization_id: 'org-1',
  media_id: 'media-1',
  media_filename: 'recording.mp3',
  media_title: 'Test Title',
  media_description: 'Test Description',
  media_status: 'ready',
  status: 'completed',
  languages: ['id'],
  diarization: true,
  speaker_count: 2,
  word_count: 100,
  duration_seconds: 60,
  created_at: '2026-02-10T10:00:00Z',
  completed_at: '2026-02-10T10:01:00Z',
}

describe('RetranscribeDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockDelete.mockResolvedValue(undefined)
    mockPost.mockResolvedValue({ id: 'tx-new' })
  })

  it('renders dialog with transcription filename', () => {
    render(
      <RetranscribeDialog
        open={true}
        onOpenChange={vi.fn()}
        transcription={mockTranscription}
      />,
    )
    expect(screen.getByText('Retranscribe Audio')).toBeInTheDocument()
    expect(screen.getByText(/recording\.mp3/)).toBeInTheDocument()
  })

  it('pre-fills diarization from transcription', () => {
    render(
      <RetranscribeDialog
        open={true}
        onOpenChange={vi.fn()}
        transcription={mockTranscription}
      />,
    )
    expect(screen.getByRole('checkbox', { name: /speaker diarization/i })).toBeChecked()
  })

  it('deletes old and creates new transcription on submit', async () => {
    const onOpenChange = vi.fn()
    const user = userEvent.setup()
    render(
      <RetranscribeDialog
        open={true}
        onOpenChange={onOpenChange}
        transcription={mockTranscription}
      />,
    )
    await user.click(screen.getByRole('button', { name: /start retranscription/i }))
    expect(mockDelete).toHaveBeenCalledWith('/transcriptions/tx-1')
    expect(mockPost).toHaveBeenCalledWith('/transcriptions', {
      media_id: 'media-1',
      languages: ['id'],
      diarization: true,
    })
    expect(toast.success).toHaveBeenCalledWith('Retranscription started')
    expect(mockNavigate).toHaveBeenCalledWith('/transcriptions/tx-new')
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('shows error toast when delete fails', async () => {
    mockDelete.mockRejectedValue(new Error('fail'))
    const user = userEvent.setup()
    render(
      <RetranscribeDialog
        open={true}
        onOpenChange={vi.fn()}
        transcription={mockTranscription}
      />,
    )
    await user.click(screen.getByRole('button', { name: /start retranscription/i }))
    expect(toast.error).toHaveBeenCalledWith(
      'Failed to delete existing transcription. No changes were made.',
    )
    expect(mockPost).not.toHaveBeenCalled()
  })

  it('shows specific error when delete succeeds but create fails', async () => {
    mockPost.mockRejectedValue(new Error('create fail'))
    const onOpenChange = vi.fn()
    const user = userEvent.setup()
    render(
      <RetranscribeDialog
        open={true}
        onOpenChange={onOpenChange}
        transcription={mockTranscription}
      />,
    )
    await user.click(screen.getByRole('button', { name: /start retranscription/i }))
    expect(mockDelete).toHaveBeenCalledWith('/transcriptions/tx-1')
    expect(toast.error).toHaveBeenCalledWith(
      'Old transcription was removed but new one failed to start. Please transcribe again from the media library.',
    )
    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(mockInvalidateQueries).toHaveBeenCalled()
  })

  // --- number of speakers ---

  it('seeds the speaker count from the original transcription and resends it', async () => {
    const user = userEvent.setup()
    render(
      <RetranscribeDialog
        open={true}
        onOpenChange={vi.fn()}
        transcription={{ ...mockTranscription, expected_speakers: 3 }}
      />,
    )
    expect(screen.getByRole('combobox', { name: 'Number of speakers' })).toHaveTextContent(
      '3 speakers',
    )

    await user.click(screen.getByRole('button', { name: /start retranscription/i }))
    expect(mockPost).toHaveBeenCalledWith('/transcriptions', {
      media_id: 'media-1',
      languages: ['id'],
      diarization: true,
      expected_speakers: 3,
    })
  })

  it('omits expected_speakers when the original left it on auto detect', async () => {
    const user = userEvent.setup()
    render(
      <RetranscribeDialog open={true} onOpenChange={vi.fn()} transcription={mockTranscription} />,
    )
    expect(screen.getByRole('combobox', { name: 'Number of speakers' })).toHaveTextContent(
      'Auto detect',
    )

    await user.click(screen.getByRole('button', { name: /start retranscription/i }))
    const [, payload] = mockPost.mock.calls[0]
    expect(payload).not.toHaveProperty('expected_speakers')
  })

  it('lets the user change the speaker count before resubmitting', async () => {
    const user = userEvent.setup()
    render(
      <RetranscribeDialog
        open={true}
        onOpenChange={vi.fn()}
        transcription={{ ...mockTranscription, expected_speakers: 3 }}
      />,
    )
    await user.click(screen.getByRole('combobox', { name: 'Number of speakers' }))
    await user.click(screen.getByRole('option', { name: '2 speakers' }))
    await user.click(screen.getByRole('button', { name: /start retranscription/i }))

    expect(mockPost).toHaveBeenCalledWith(
      '/transcriptions',
      expect.objectContaining({ expected_speakers: 2 }),
    )
  })

  it('hides the speaker count and drops it when diarization is turned off', async () => {
    const user = userEvent.setup()
    render(
      <RetranscribeDialog
        open={true}
        onOpenChange={vi.fn()}
        transcription={{ ...mockTranscription, expected_speakers: 3 }}
      />,
    )
    await user.click(screen.getByLabelText(/speaker diarization/i))
    expect(
      screen.queryByRole('combobox', { name: 'Number of speakers' }),
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /start retranscription/i }))
    const [, payload] = mockPost.mock.calls[0]
    expect(payload).not.toHaveProperty('expected_speakers')
  })

  it('cancel button is present', () => {
    render(
      <RetranscribeDialog
        open={true}
        onOpenChange={vi.fn()}
        transcription={mockTranscription}
      />,
    )
    expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument()
  })
})
