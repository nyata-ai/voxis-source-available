import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LanguageBadge } from './LanguageBadge'

describe('LanguageBadge', () => {
  it('renders uppercase codes joined by a middle dot for an explicit multi-language request', () => {
    render(<LanguageBadge languages={['id', 'en']} />)
    expect(screen.getByText('ID·EN')).toBeInTheDocument()
  })

  it('renders a single uppercase code', () => {
    render(<LanguageBadge languages={['id']} />)
    expect(screen.getByText('ID')).toBeInTheDocument()
  })

  it('renders nothing when the only language is auto', () => {
    const { container } = render(<LanguageBadge languages={['auto']} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when languages is empty', () => {
    const { container } = render(<LanguageBadge languages={[]} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when any member is auto, even mixed with an explicit language', () => {
    const { container } = render(<LanguageBadge languages={['auto', 'en']} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('has an aria-label', () => {
    render(<LanguageBadge languages={['id', 'en']} />)
    const badge = screen.getByText('ID·EN')
    expect(badge).toHaveAttribute('aria-label')
    expect(badge.getAttribute('aria-label')?.length).toBeGreaterThan(0)
  })
})
