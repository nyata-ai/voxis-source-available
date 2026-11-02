import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { StatusChip } from './status-chip'

describe('StatusChip', () => {
  it('renders the translated label for a known status', () => {
    render(<StatusChip status="scan_pending" />)
    expect(screen.getByText('Scan pending')).toBeInTheDocument()
  })

  it('falls back to the raw status string for an unknown status', () => {
    render(<StatusChip status="some_unknown_status" />)
    expect(screen.getByText('some_unknown_status')).toBeInTheDocument()
  })

  it('adds a pulsing dot only for live tones', () => {
    const { container: liveContainer } = render(<StatusChip status="pending" />)
    expect(liveContainer.querySelector('.status-pulse')).toBeInTheDocument()

    const { container: neutralContainer } = render(<StatusChip status="completed" />)
    expect(neutralContainer.querySelector('.status-pulse')).not.toBeInTheDocument()
  })

  it('respects a label override instead of the translated status', () => {
    render(<StatusChip status="ready" label="Custom label" />)
    expect(screen.getByText('Custom label')).toBeInTheDocument()
    expect(screen.queryByText('Ready')).not.toBeInTheDocument()
  })

  it('sets the label at 12px semibold in sentence case, not shouted caps', () => {
    render(<StatusChip status="scan_pending" />)
    const chip = screen.getByText('Scan pending')
    expect(chip).toHaveClass('text-caption', 'font-semibold')
    expect(chip).not.toHaveClass('uppercase')
  })
})
