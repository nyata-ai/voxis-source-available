import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { StepTrack } from './StepTrack'

function dots(): Element[] {
  return [...screen.getByTestId('step-track').querySelectorAll('[data-step]')]
}

describe('StepTrack', () => {
  it('draws one dot per step and names only the current step', () => {
    render(<StepTrack steps={['done', 'current', 'pending']} label="Encrypting" />)

    expect(dots().map((dot) => dot.getAttribute('data-step'))).toEqual(['done', 'current', 'pending'])
    expect(screen.getByText('Encrypting')).toBeInTheDocument()
  })

  it('hides the dots from assistive technology so the text is the label', () => {
    render(<StepTrack steps={['current', 'pending']} label="Uploading" />)

    expect(screen.getByTestId('step-track')).toHaveAttribute('aria-hidden', 'true')
  })

  it('pulses the current step only when motion is allowed and fills done steps over 240ms', () => {
    render(<StepTrack steps={['done', 'current']} label="Scanning" />)
    const [done, current] = dots()

    expect(current.className).toContain('motion-safe:animate-[status-pulse_1.8s_ease-in-out_infinite]')
    expect(done).toHaveClass('bg-current', 'duration-240', 'motion-reduce:transition-none')
  })

  it('uses the error colour for a failed step, never coral', () => {
    const { rerender } = render(<StepTrack steps={['done', 'failed']} label="Failed" />)
    expect(dots()[1]).toHaveClass('bg-destructive')

    rerender(<StepTrack steps={['done', 'failed']} label="Failed" surface="ink" />)
    expect(dots()[1]).toHaveClass('bg-destructive-inverse')
  })
})
