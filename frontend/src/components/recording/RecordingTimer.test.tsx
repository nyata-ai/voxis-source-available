import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RecordingTimer } from './RecordingTimer'

describe('RecordingTimer', () => {
  it('displays 00:00:00 for zero elapsed', () => {
    render(<RecordingTimer elapsed={0} isPaused={false} />)
    expect(screen.getByText('00:00:00')).toBeInTheDocument()
  })

  it('formats seconds correctly', () => {
    render(<RecordingTimer elapsed={5} isPaused={false} />)
    expect(screen.getByText('00:00:05')).toBeInTheDocument()
  })

  it('formats minutes and seconds correctly', () => {
    render(<RecordingTimer elapsed={125} isPaused={false} />)
    expect(screen.getByText('00:02:05')).toBeInTheDocument()
  })

  it('formats hours, minutes, and seconds correctly', () => {
    render(<RecordingTimer elapsed={3661} isPaused={false} />)
    expect(screen.getByText('01:01:01')).toBeInTheDocument()
  })

  it('has role="timer"', () => {
    render(<RecordingTimer elapsed={0} isPaused={false} />)
    expect(screen.getByRole('timer')).toBeInTheDocument()
  })

  it('has aria-label with elapsed time', () => {
    render(<RecordingTimer elapsed={90} isPaused={false} />)
    expect(screen.getByRole('timer')).toHaveAttribute(
      'aria-label',
      'Elapsed time: 00:01:30',
    )
  })

  it('applies muted style when paused', () => {
    const { container } = render(<RecordingTimer elapsed={10} isPaused={true} />)
    const span = container.querySelector('span')
    expect(span?.className).toContain('text-muted-foreground')
  })

  it('applies foreground style when not paused', () => {
    const { container } = render(<RecordingTimer elapsed={10} isPaused={false} />)
    const span = container.querySelector('span')
    expect(span?.className).toContain('text-foreground')
  })
})
