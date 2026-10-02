import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { PaginationControls } from './PaginationControls'

describe('PaginationControls', () => {
  it('renders page info', () => {
    render(
      <PaginationControls offset={0} limit={10} total={25} onPageChange={vi.fn()} />,
    )
    expect(screen.getByText(/Showing 1/)).toBeInTheDocument()
    expect(screen.getByText('1 / 3')).toBeInTheDocument()
  })

  it('disables prev on first page', () => {
    render(
      <PaginationControls offset={0} limit={10} total={25} onPageChange={vi.fn()} />,
    )
    expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled()
  })

  it('disables next on last page', () => {
    render(
      <PaginationControls offset={20} limit={10} total={25} onPageChange={vi.fn()} />,
    )
    expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled()
  })

  it('calls onPageChange when next is clicked', async () => {
    const user = userEvent.setup()
    const onPageChange = vi.fn()
    render(
      <PaginationControls offset={0} limit={10} total={25} onPageChange={onPageChange} />,
    )
    await user.click(screen.getByRole('button', { name: 'Next page' }))
    expect(onPageChange).toHaveBeenCalledWith(10)
  })

  it('calls onPageChange when prev is clicked', async () => {
    const user = userEvent.setup()
    const onPageChange = vi.fn()
    render(
      <PaginationControls offset={10} limit={10} total={25} onPageChange={onPageChange} />,
    )
    await user.click(screen.getByRole('button', { name: 'Previous page' }))
    expect(onPageChange).toHaveBeenCalledWith(0)
  })

  it('returns null when total <= limit', () => {
    const { container } = render(
      <PaginationControls offset={0} limit={10} total={5} onPageChange={vi.fn()} />,
    )
    expect(container).toBeEmptyDOMElement()
  })
})
