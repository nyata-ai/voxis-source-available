import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TranscribeButton } from './TranscribeButton'

const mockMutate = vi.fn()
const mockUseCreateTranscription = vi.fn()
const mockUseFeatures = vi.fn()

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}))

vi.mock('@/hooks/useTranscription', () => ({
  useCreateTranscription: () => mockUseCreateTranscription(),
}))

vi.mock('@/hooks/useFeatures', () => ({
  useFeatures: () => mockUseFeatures(),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

describe('TranscribeButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseCreateTranscription.mockReturnValue({ mutate: mockMutate, isPending: false })
  })

  it('does not offer or submit vocabulary packs when a stale capability says they exist', async () => {
    mockUseFeatures.mockReturnValue({ data: { custom_vocabulary: true } })
    const user = userEvent.setup()

    render(<TranscribeButton mediaId="m-1" mediaStatus="ready" />)
    await user.click(screen.getByRole('button', { name: /transcribe/i }))

    expect(screen.queryByText('Terminology (optional)')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /start transcription/i }))

    const [request] = mockMutate.mock.calls[0]
    expect(request).not.toHaveProperty('vocabulary_packs')
  })
})
