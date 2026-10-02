import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { DeleteConfirmDialog } from './DeleteConfirmDialog'

describe('DeleteConfirmDialog', () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    filename: 'test-audio.mp3',
    onConfirm: vi.fn(),
  }

  it('renders filename in description', () => {
    render(<DeleteConfirmDialog {...defaultProps} />)
    expect(screen.getByText('test-audio.mp3')).toBeInTheDocument()
  })

  it('renders delete and cancel buttons', () => {
    render(<DeleteConfirmDialog {...defaultProps} />)
    expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
  })

  it('calls onConfirm when delete is clicked', async () => {
    const user = userEvent.setup()
    render(<DeleteConfirmDialog {...defaultProps} />)
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    expect(defaultProps.onConfirm).toHaveBeenCalled()
  })

  it('shows deleting state', () => {
    render(<DeleteConfirmDialog {...defaultProps} isDeleting />)
    expect(screen.getByRole('button', { name: 'Deleting...' })).toBeDisabled()
  })

  it('does not render when closed', () => {
    render(<DeleteConfirmDialog {...defaultProps} open={false} />)
    expect(screen.queryByText('Delete media file?')).not.toBeInTheDocument()
  })
})
