import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AdminStorageQuotaPanel } from './AdminStorageQuotaPanel'

const mockUpdateMutateAsync = vi.fn()

vi.mock('@/hooks/useAdminStorageQuota', () => ({
  useAdminStorageQuota: vi.fn(),
  useUpdateAdminStorageQuota: vi.fn(() => ({
    mutateAsync: mockUpdateMutateAsync,
    isPending: false,
  })),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { useAdminStorageQuota } from '@/hooks/useAdminStorageQuota'

const GIB = 1024 ** 3

describe('AdminStorageQuotaPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useAdminStorageQuota).mockReturnValue({
      data: {
        enabled: false,
        default_limit_bytes: 2 * GIB,
        warning_threshold_percent: 85,
        backend: 'gcs',
        applies: true,
      },
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useAdminStorageQuota>)
    mockUpdateMutateAsync.mockResolvedValue({})
  })

  it('shows the default 2 GiB quota and fixed warning threshold', () => {
    render(<AdminStorageQuotaPanel />)

    expect(screen.getByRole('switch', { name: 'Enforce storage quota' })).toHaveAttribute(
      'data-state',
      'unchecked',
    )
    expect(screen.getByLabelText('Default limit (GiB)')).toHaveValue(2)
    expect(screen.getByText('85%')).toBeInTheDocument()
    expect(screen.getByText('gcs')).toBeInTheDocument()
  })

  it('saves a positive fractional GiB limit', async () => {
    const user = userEvent.setup()
    render(<AdminStorageQuotaPanel />)

    await user.click(screen.getByRole('switch', { name: 'Enforce storage quota' }))
    await user.clear(screen.getByLabelText('Default limit (GiB)'))
    await user.type(screen.getByLabelText('Default limit (GiB)'), '1.5')
    await user.click(screen.getByRole('button', { name: 'Save storage quota' }))

    expect(mockUpdateMutateAsync).toHaveBeenCalledWith({
      enabled: true,
      default_limit_bytes: 1.5 * GIB,
    })
  })

  it('preserves exact existing bytes when only enforcement changes', async () => {
    const user = userEvent.setup()
    vi.mocked(useAdminStorageQuota).mockReturnValue({
      data: {
        enabled: false,
        default_limit_bytes: 4096,
        warning_threshold_percent: 85,
        backend: 'gcs',
        applies: true,
      },
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useAdminStorageQuota>)

    render(<AdminStorageQuotaPanel />)
    await user.click(screen.getByRole('switch', { name: 'Enforce storage quota' }))
    await user.click(screen.getByRole('button', { name: 'Save storage quota' }))

    expect(mockUpdateMutateAsync).toHaveBeenCalledWith({
      enabled: true,
      default_limit_bytes: 4096,
    })
  })

  it('does not save a zero limit', async () => {
    const user = userEvent.setup()
    render(<AdminStorageQuotaPanel />)

    await user.click(screen.getByRole('switch', { name: 'Enforce storage quota' }))
    await user.clear(screen.getByLabelText('Default limit (GiB)'))
    await user.type(screen.getByLabelText('Default limit (GiB)'), '0')

    expect(screen.getByText('Enter a positive storage limit.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save storage quota' })).toBeDisabled()
  })

  it('accepts a limit above 1,024 GiB', async () => {
    const user = userEvent.setup()
    render(<AdminStorageQuotaPanel />)

    await user.click(screen.getByRole('switch', { name: 'Enforce storage quota' }))
    await user.clear(screen.getByLabelText('Default limit (GiB)'))
    await user.type(screen.getByLabelText('Default limit (GiB)'), '2048')
    await user.click(screen.getByRole('button', { name: 'Save storage quota' }))

    expect(mockUpdateMutateAsync).toHaveBeenCalledWith({
      enabled: true,
      default_limit_bytes: 2048 * GIB,
    })
  })

  it('honestly marks localfs storage inactive', () => {
    vi.mocked(useAdminStorageQuota).mockReturnValue({
      data: {
        enabled: true,
        default_limit_bytes: 2 * GIB,
        warning_threshold_percent: 85,
        backend: 'localfs',
        applies: false,
      },
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useAdminStorageQuota>)

    render(<AdminStorageQuotaPanel />)

    expect(screen.getByText('Inactive: quotas are not enforced with localfs storage.')).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'Enforce storage quota' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Save storage quota' })).toBeDisabled()
  })
})
