import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ErrorFallback } from './ErrorFallback'

describe('ErrorFallback', () => {
  it('displays generic error message', () => {
    const error = new Error('Test error message')
    const resetErrorBoundary = vi.fn()

    render(<ErrorFallback error={error} resetErrorBoundary={resetErrorBoundary} />)

    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    expect(screen.getByText('An unexpected error occurred. Please try again.')).toBeInTheDocument()
  })

  it('calls resetErrorBoundary when try again is clicked', async () => {
    const user = userEvent.setup()
    const error = new Error('Test error')
    const resetErrorBoundary = vi.fn()

    render(<ErrorFallback error={error} resetErrorBoundary={resetErrorBoundary} />)

    await user.click(screen.getByRole('button', { name: /try again/i }))

    expect(resetErrorBoundary).toHaveBeenCalledOnce()
  })

  it('has accessible alert role', () => {
    const error = new Error('Test error')
    const resetErrorBoundary = vi.fn()

    render(<ErrorFallback error={error} resetErrorBoundary={resetErrorBoundary} />)

    expect(screen.getByRole('alert')).toBeInTheDocument()
  })

  it('shows error details in development mode', () => {
    const originalEnv = import.meta.env.DEV
    ;(import.meta.env as Record<string, unknown>).DEV = true

    const error = new Error('Detailed error message')
    const resetErrorBoundary = vi.fn()

    render(<ErrorFallback error={error} resetErrorBoundary={resetErrorBoundary} />)

    // In dev mode, error details should be visible
    expect(screen.getByText('Error details (dev only)')).toBeInTheDocument()
    expect(screen.getByText('Detailed error message')).toBeInTheDocument()

    ;(import.meta.env as Record<string, unknown>).DEV = originalEnv
  })

  it('handles non-Error objects gracefully', () => {
    const originalEnv = import.meta.env.DEV
    ;(import.meta.env as Record<string, unknown>).DEV = true

    const error = 'String error'
    const resetErrorBoundary = vi.fn()

    render(<ErrorFallback error={error} resetErrorBoundary={resetErrorBoundary} />)

    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    // The string should be converted via String()
    expect(screen.getByText('String error')).toBeInTheDocument()

    ;(import.meta.env as Record<string, unknown>).DEV = originalEnv
  })

  it('renders refresh icon in button', () => {
    const error = new Error('Test error')
    const resetErrorBoundary = vi.fn()

    const { container } = render(
      <ErrorFallback error={error} resetErrorBoundary={resetErrorBoundary} />
    )

    // Check for the icon with aria-hidden
    const icon = container.querySelector('button svg[aria-hidden="true"]')
    expect(icon).toBeInTheDocument()
  })

  it('renders alert icon with aria-hidden', () => {
    const error = new Error('Test error')
    const resetErrorBoundary = vi.fn()

    const { container } = render(
      <ErrorFallback error={error} resetErrorBoundary={resetErrorBoundary} />
    )

    // The AlertTriangle icon should have aria-hidden
    const icons = container.querySelectorAll('svg[aria-hidden="true"]')
    expect(icons.length).toBeGreaterThanOrEqual(1)
  })
})
