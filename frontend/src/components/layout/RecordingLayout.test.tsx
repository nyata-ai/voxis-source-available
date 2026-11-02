import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
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

  it('renders branding, a Cancel button, and children', () => {
    render(
      <MemoryRouter>
        <RecordingLayout>
          <div>Recording child</div>
        </RecordingLayout>
      </MemoryRouter>
    )

    expect(screen.getByText('Voxis Source-Available')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
    expect(screen.getByText('Recording child')).toBeInTheDocument()
  })

  it('Cancel goes back to the page the user came from', async () => {
    render(
      <MemoryRouter initialEntries={['/dashboard', '/record']} initialIndex={1}>
        <Routes>
          <Route path="/dashboard" element={<div>Desk page</div>} />
          <Route
            path="/record"
            element={
              <RecordingLayout>
                <div>Recording child</div>
              </RecordingLayout>
            }
          />
          <Route path="/library" element={<div>Library page</div>} />
        </Routes>
      </MemoryRouter>,
    )

    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(await screen.findByText('Desk page')).toBeInTheDocument()
  })

  it('Cancel falls back to the Library when the record page was opened directly', async () => {
    render(
      <MemoryRouter initialEntries={['/record']}>
        <Routes>
          <Route
            path="/record"
            element={
              <RecordingLayout>
                <div>Recording child</div>
              </RecordingLayout>
            }
          />
          <Route path="/library" element={<div>Library page</div>} />
        </Routes>
      </MemoryRouter>,
    )

    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(await screen.findByText('Library page')).toBeInTheDocument()
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
