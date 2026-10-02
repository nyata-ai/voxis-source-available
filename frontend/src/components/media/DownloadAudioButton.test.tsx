import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { DownloadAudioButton } from './DownloadAudioButton'

vi.mock('@/lib/api-client', () => ({
  apiClient: {
    download: vi.fn(),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { apiClient } from '@/lib/api-client'
import { toast } from '@/lib/toast'

describe('DownloadAudioButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders button for completed transcription', () => {
    render(<DownloadAudioButton mediaId="media-1" mediaFilename="meeting.mp3" />)
    expect(screen.getByRole('button', { name: /download audio/i })).toBeInTheDocument()
  })

  it('does not render when status is not completed', () => {
    const { container } = render(
      <DownloadAudioButton mediaId="media-1" mediaFilename="meeting.mp3" status="pending" />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('does not render when audio is unavailable', () => {
    const { container } = render(
      <DownloadAudioButton mediaId="media-1" mediaFilename="meeting.mp3" audioAvailable={false} />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('downloads original audio', async () => {
    const user = userEvent.setup()
    vi.mocked(apiClient.download).mockResolvedValueOnce(undefined)

    render(<DownloadAudioButton mediaId="media-1" mediaFilename="meeting.mp3" />)

    await user.click(screen.getByRole('button', { name: /download audio/i }))

    expect(apiClient.download).toHaveBeenCalledWith('/media/media-1/download', 'meeting.mp3')
    expect(toast.success).toHaveBeenCalledWith('Audio downloaded')
  })

  it('shows error toast when download fails', async () => {
    const user = userEvent.setup()
    vi.mocked(apiClient.download).mockRejectedValueOnce(new Error('Network error'))

    render(<DownloadAudioButton mediaId="media-1" mediaFilename="meeting.mp3" />)

    await user.click(screen.getByRole('button', { name: /download audio/i }))

    expect(toast.error).toHaveBeenCalledWith('Failed to download audio. Please try again.')
  })
})
