import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AdminRecordingRetentionPanel } from './AdminRecordingRetentionPanel'

const mockPreviewMutateAsync = vi.fn()
const mockUpdateMutateAsync = vi.fn()

vi.mock('@/hooks/useAdminRecordingRetention', () => ({
  useAdminRecordingRetentionPolicy: vi.fn(),
  useAdminRecordingRetentionPreview: vi.fn(() => ({
    mutateAsync: mockPreviewMutateAsync,
    isPending: false,
  })),
  useUpdateAdminRecordingRetentionPolicy: vi.fn(() => ({
    mutateAsync: mockUpdateMutateAsync,
    isPending: false,
  })),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { useAdminRecordingRetentionPolicy } from '@/hooks/useAdminRecordingRetention'

describe('AdminRecordingRetentionPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useAdminRecordingRetentionPolicy).mockReturnValue({
      data: {
        enabled: false,
        days: 7,
        apply_to_existing: false,
      },
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useAdminRecordingRetentionPolicy>)
    mockPreviewMutateAsync.mockResolvedValue({
      immediate_delete_count: 0,
      prospective_effective_at: '2026-06-30T00:00:00Z',
    })
    mockUpdateMutateAsync.mockResolvedValue({
      enabled: true,
      days: 14,
      apply_to_existing: false,
    })
  })

  it('renders the current global retention policy', async () => {
    render(<AdminRecordingRetentionPanel />)

    expect(screen.getByRole('switch', { name: 'Auto-delete live recording audio' })).toHaveAttribute(
      'data-state',
      'unchecked',
    )
    expect(screen.getByLabelText('Delete after')).toHaveValue(7)
    await waitFor(() => {
      expect(mockPreviewMutateAsync).toHaveBeenCalled()
    })
  })

  it('shows an error instead of placeholders when the policy cannot load', () => {
    vi.mocked(useAdminRecordingRetentionPolicy).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('failed'),
    } as ReturnType<typeof useAdminRecordingRetentionPolicy>)

    render(<AdminRecordingRetentionPanel />)

    expect(screen.getByText('Unable to load recording retention policy.')).toBeInTheDocument()
    expect(screen.queryByTestId('skeleton')).not.toBeInTheDocument()
    expect(screen.queryByRole('switch', { name: 'Auto-delete live recording audio' })).not.toBeInTheDocument()
  })

  it('previews and confirms immediate deletion before saving retroactive policy', async () => {
    const user = userEvent.setup()
    mockPreviewMutateAsync.mockResolvedValue({
      immediate_delete_count: 3,
      oldest_completed_at: '2026-06-01T00:00:00Z',
      prospective_effective_at: '2026-06-30T00:00:00Z',
    })

    render(<AdminRecordingRetentionPanel />)

    await user.click(screen.getByRole('switch', { name: 'Auto-delete live recording audio' }))
    await user.clear(screen.getByLabelText('Delete after'))
    await user.type(screen.getByLabelText('Delete after'), '14')
    await user.click(screen.getByRole('checkbox', { name: 'Apply to existing live recordings' }))

    await waitFor(() => {
      expect(screen.getByText('3 live recordings have audio eligible for immediate deletion.')).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: 'Save retention policy' }))
    expect(mockUpdateMutateAsync).not.toHaveBeenCalled()

    await user.type(screen.getByLabelText('Confirm immediate delete count'), '3')
    await user.click(screen.getByRole('button', { name: 'Save retention policy' }))

    expect(mockUpdateMutateAsync).toHaveBeenCalledWith({
      enabled: true,
      days: 14,
      apply_to_existing: true,
      confirmed_immediate_delete_count: 3,
    })
  })
})
