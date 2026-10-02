import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { InlineEditableText } from './InlineEditableText'

describe('InlineEditableText', () => {
  it('renders value as text by default', () => {
    render(<InlineEditableText value="Meeting" onSave={vi.fn()} />)
    expect(screen.getByText('Meeting')).toBeInTheDocument()
  })

  it('shows placeholder when value is empty', () => {
    render(<InlineEditableText value="" placeholder="Add title..." onSave={vi.fn()} />)
    expect(screen.getByText('Add title...')).toBeInTheDocument()
  })

  it('switches to input on click', async () => {
    const user = userEvent.setup()
    render(<InlineEditableText value="Meeting" onSave={vi.fn()} />)
    await user.click(screen.getByText('Meeting'))
    expect(screen.getByRole('textbox')).toHaveValue('Meeting')
  })

  it('calls onSave on blur with new value', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<InlineEditableText value="Old" onSave={onSave} />)
    await user.click(screen.getByText('Old'))
    const input = screen.getByRole('textbox')
    await user.clear(input)
    await user.type(input, 'New')
    await user.tab() // triggers blur
    expect(onSave).toHaveBeenCalledWith('New')
  })

  it('calls onSave on Enter', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<InlineEditableText value="Old" onSave={onSave} />)
    await user.click(screen.getByText('Old'))
    const input = screen.getByRole('textbox')
    expect(input).toHaveFocus()
    expect(input).toHaveProperty('selectionStart', 0)
    expect(input).toHaveProperty('selectionEnd', 3)
    await user.clear(input)
    await user.type(input, 'New{Enter}')
    expect(onSave).toHaveBeenCalledWith('New')
  })

  it('reverts on Escape without saving', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<InlineEditableText value="Original" onSave={onSave} />)
    await user.click(screen.getByText('Original'))
    const input = screen.getByRole('textbox')
    await user.clear(input)
    await user.type(input, 'Changed')
    await user.keyboard('{Escape}')
    expect(onSave).not.toHaveBeenCalled()
    expect(screen.getByText('Original')).toBeInTheDocument()
  })

  it('does not call onSave if value unchanged', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<InlineEditableText value="Same" onSave={onSave} />)
    await user.click(screen.getByText('Same'))
    await user.tab()
    expect(onSave).not.toHaveBeenCalled()
  })

  it('does not double-save on Enter (Enter + blur)', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(<InlineEditableText value="Old" onSave={onSave} />)
    await user.click(screen.getByText('Old'))
    const input = screen.getByRole('textbox')
    await user.clear(input)
    await user.type(input, 'New{Enter}')
    expect(onSave).toHaveBeenCalledTimes(1)
  })
})
