import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import type { ReactNode } from 'react'
import { RecordingLayout } from './RecordingLayout'

let mockProtectedShouldRender = true

vi.mock('@/components/auth/ProtectedRoute', () => ({
  ProtectedRoute: ({ children }: { children: ReactNode }) =>
    mockProtectedShouldRender ? <>{children}</> : <div>Checking access</div>,
}))

describe('RecordingLayout', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockProtectedShouldRender = true
  })

  it('renders branding, cancel link, and children', () => {
    render(
      <MemoryRouter>
        <RecordingLayout>
          <div>Recording child</div>
        </RecordingLayout>
      </MemoryRouter>
    )

    expect(screen.getByText('Voxis Source-Available')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Cancel' })).toHaveAttribute('href', '/library')
    expect(screen.getByText('Recording child')).toBeInTheDocument()
  })

  // RecordingLayout is deliberately a separate, minimal layout for the
  // full-page recording lockout — it never imports ConfidentialityStrip.
  // This locks in that split so the strip doesn't silently reappear here.
  it('does not render the app-wide confidentiality strip', () => {
    render(
      <MemoryRouter>
        <RecordingLayout>
          <div>Recording child</div>
        </RecordingLayout>
      </MemoryRouter>
    )

    expect(screen.queryByRole('note', { name: 'Data protection' })).not.toBeInTheDocument()
  })

  it('renders the access check instead of children when protection denies', () => {
    mockProtectedShouldRender = false

    render(
      <MemoryRouter>
        <RecordingLayout>
          <div>Recording child</div>
        </RecordingLayout>
      </MemoryRouter>
    )

    expect(screen.getByText('Checking access')).toBeInTheDocument()
    expect(screen.queryByText('Recording child')).not.toBeInTheDocument()
  })

  // Language sync moved to App, above the router — asserting it here would only
  // re-test a hook this layout no longer owns.
})
