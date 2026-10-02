import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'

const mockLogin = vi.fn()
const mockLogout = vi.fn().mockResolvedValue(undefined)
const mockUseAuth = vi.fn()

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => mockUseAuth(),
}))

import { useAccessStore } from '@/stores/access'
import { ProtectedRoute } from './ProtectedRoute'

function TestComponent() {
  return <div>Protected Content</div>
}

function renderWithRouter(
  isAuthenticated: boolean,
  isLoading: boolean,
  error: string | null = null,
  isLoggingOut = false,
) {
  mockUseAuth.mockReturnValue({
    isAuthenticated,
    isLoading,
    isLoggingOut,
    user: isAuthenticated
      ? { id: '1', email: 'test@example.com', name: 'Test', username: 'test' }
      : null,
    token: isAuthenticated ? 'token' : undefined,
    error,
    login: mockLogin,
    register: vi.fn(),
    logout: mockLogout,
    updateToken: vi.fn(),
  })

  return render(
    <MemoryRouter initialEntries={['/protected']}>
      <Routes>
        <Route
          path="/protected"
          element={
            <ProtectedRoute>
              <TestComponent />
            </ProtectedRoute>
          }
        />
      </Routes>
    </MemoryRouter>
  )
}

describe('ProtectedRoute', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('should show loading state while auth is initializing', () => {
    renderWithRouter(false, true)

    expect(screen.getByText(/loading/i)).toBeInTheDocument()
    expect(screen.queryByText('Protected Content')).not.toBeInTheDocument()
  })

  it('should render children when authenticated', () => {
    renderWithRouter(true, false)

    expect(screen.getByText('Protected Content')).toBeInTheDocument()
  })

  it('should trigger login when not authenticated', () => {
    renderWithRouter(false, false)

    expect(mockLogin).toHaveBeenCalled()
    expect(screen.getByText(/redirecting/i)).toBeInTheDocument()
  })

  it('should show generic error message when auth fails', () => {
    renderWithRouter(false, false, 'Some technical error')

    // Should show generic message, not the actual error
    expect(screen.getByText(/unable to authenticate/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /try again/i })).toBeInTheDocument()
  })

  it('should only trigger login once even with multiple renders', () => {
    const { rerender } = renderWithRouter(false, false)

    // Re-render the component
    rerender(
      <MemoryRouter initialEntries={['/protected']}>
        <Routes>
          <Route
            path="/protected"
            element={
              <ProtectedRoute>
                <TestComponent />
              </ProtectedRoute>
            }
          />
        </Routes>
      </MemoryRouter>
    )

    // Login should only be called once due to the ref guard
    expect(mockLogin).toHaveBeenCalledTimes(1)
  })

  it('should show loading spinner during loading', () => {
    const { container } = renderWithRouter(false, true)

    const spinner = container.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  it('should show error card with title when error occurs', () => {
    renderWithRouter(false, false, 'Authentication failed')

    expect(screen.getByText('Authentication Error')).toBeInTheDocument()
  })

  it('should reload page when try again is clicked', () => {
    const mockReload = vi.fn()
    const originalLocation = window.location

    // Mock window.location.reload
    Object.defineProperty(window, 'location', {
      value: { ...originalLocation, reload: mockReload },
      writable: true,
    })

    renderWithRouter(false, false, 'Authentication failed')

    const tryAgainButton = screen.getByRole('button', { name: /try again/i })
    fireEvent.click(tryAgainButton)

    expect(mockReload).toHaveBeenCalled()

    // Restore
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
    })
  })

  it('should show redirect message when not authenticated and no error', () => {
    renderWithRouter(false, false)

    expect(screen.getByText(/redirecting to login/i)).toBeInTheDocument()
  })

  it('should not call login when there is an error', () => {
    renderWithRouter(false, false, 'Error occurred')

    // Login should not be called when there is an error
    expect(mockLogin).not.toHaveBeenCalled()
  })

  it('should not trigger login while logout is in progress', () => {
    renderWithRouter(false, false, null, true)

    expect(mockLogin).not.toHaveBeenCalled()
  })

  it('should not show protected content during loading', () => {
    renderWithRouter(false, true)

    expect(screen.queryByText('Protected Content')).not.toBeInTheDocument()
  })

  it('should not show protected content when there is an error', () => {
    renderWithRouter(false, false, 'Error')

    expect(screen.queryByText('Protected Content')).not.toBeInTheDocument()
  })

  describe('account without the app role', () => {
    afterEach(() => {
      useAccessStore.setState({ roleRequired: false })
    })

    it('shows an ask-your-administrator screen instead of the page', () => {
      useAccessStore.setState({ roleRequired: true })
      renderWithRouter(true, false)

      expect(screen.queryByText('Protected Content')).not.toBeInTheDocument()
      expect(screen.getByRole('alert')).toHaveTextContent(/administrator/i)
      expect(mockLogin).not.toHaveBeenCalled()
    })

    it('offers sign-out rather than another login attempt', () => {
      useAccessStore.setState({ roleRequired: true })
      renderWithRouter(true, false)

      fireEvent.click(screen.getByRole('button', { name: /sign out/i }))

      expect(mockLogout).toHaveBeenCalledTimes(1)
      expect(mockLogin).not.toHaveBeenCalled()
    })
  })
})
