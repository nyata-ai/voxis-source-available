import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Mic } from 'lucide-react'
import { HeaderAction } from './HeaderAction'

describe('HeaderAction', () => {
  // jsdom applies no media queries, so the `hidden` label is not what names it.
  it('keeps its name when the label is hidden at small widths', async () => {
    const onClick = vi.fn()
    render(<HeaderAction icon={Mic} label="Record" onClick={onClick} />)

    const button = screen.getByRole('button', { name: 'Record' })
    expect(button).toHaveAttribute('aria-label', 'Record')

    await userEvent.click(button)
    expect(onClick).toHaveBeenCalledTimes(1)
  })
})
