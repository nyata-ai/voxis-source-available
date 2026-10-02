import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { PlaceholderPage } from './PlaceholderPage'

describe('PlaceholderPage', () => {
  it('renders title prop', () => {
    render(<PlaceholderPage title="Transcriptions" />)

    expect(screen.getByText('Transcriptions')).toBeInTheDocument()
  })

  it('renders default coming soon message when no description provided', () => {
    render(<PlaceholderPage title="Settings" />)

    expect(
      screen.getByText('This feature is coming soon. Check back later!')
    ).toBeInTheDocument()
  })

  it('renders custom description when provided', () => {
    render(
      <PlaceholderPage
        title="Billing"
        description="View and manage your billing"
      />
    )

    expect(screen.getByText('View and manage your billing')).toBeInTheDocument()
    expect(
      screen.queryByText('This feature is coming soon. Check back later!')
    ).not.toBeInTheDocument()
  })

  it('renders construction icon with aria-hidden', () => {
    const { container } = render(<PlaceholderPage title="Test" />)

    const icon = container.querySelector('svg[aria-hidden="true"]')
    expect(icon).toBeInTheDocument()
  })

  it('renders working hard message', () => {
    render(<PlaceholderPage title="Test" />)

    expect(
      screen.getByText("We're working hard to bring you this feature.")
    ).toBeInTheDocument()
  })

  it('renders in a card layout', () => {
    const { container } = render(<PlaceholderPage title="Test" />)

    // The component uses a Card component with text-center class
    const card = container.querySelector('.text-center')
    expect(card).toBeInTheDocument()
  })

  it('renders different titles correctly', () => {
    const { rerender } = render(<PlaceholderPage title="Settings" />)
    expect(screen.getByText('Settings')).toBeInTheDocument()

    rerender(<PlaceholderPage title="Billing" />)
    expect(screen.getByText('Billing')).toBeInTheDocument()
    expect(screen.queryByText('Settings')).not.toBeInTheDocument()
  })
})
