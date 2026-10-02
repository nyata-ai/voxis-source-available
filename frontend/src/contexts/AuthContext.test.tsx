import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18n from '../i18n'

vi.mock('../router', () => ({
  router: {
    navigate: vi.fn(),
  },
}))

// Mock keycloak module - must be inlined for hoisting
vi.mock('../lib/keycloak', () => {
  const mockKeycloak = {
    init: vi.fn().mockResolvedValue(true),
    login: vi.fn().mockResolvedValue(undefined),
    logout: vi.fn().mockResolvedValue(undefined),
    updateToken: vi.fn().mockResolvedValue(true),
    authenticated: true,
    token: 'mock-token',
    tokenParsed: {
      sub: 'user-123',
      email: 'test@example.com',
      preferred_username: 'testuser',
      name: 'Test User',
    } as Record<string, unknown> | undefined,
    idTokenParsed: {
      sub: 'user-123',
      email: 'test@example.com',
      preferred_username: 'testuser',
      name: 'Test User',
    } as Record<string, unknown> | undefined,
    onReady: null as ((authenticated: boolean) => void) | null,
    onAuthSuccess: null as (() => void) | null,
    onAuthError: null as ((error: unknown) => void) | null,
    onTokenExpired: null as (() => void) | null,
  }
  return {
    keycloak: mockKeycloak,
    initOptions: { onLoad: 'check-sso' },
  }
})

vi.mock('../lib/chunk-outbox', () => ({
  claimOutbox: vi.fn().mockResolvedValue(undefined),
  wipeOutbox: vi.fn().mockResolvedValue(undefined),
}))

// Import after mock setup
import { keycloak } from '../lib/keycloak'
import { claimOutbox, wipeOutbox } from '../lib/chunk-outbox'
import { useAuth } from './AuthContext'
import { AuthProvider } from './AuthProvider'

// Type the mocked keycloak for test access
const mockKeycloak = keycloak as unknown as {
  init: ReturnType<typeof vi.fn>
  login: ReturnType<typeof vi.fn>
  logout: ReturnType<typeof vi.fn>
  updateToken: ReturnType<typeof vi.fn>
  authenticated: boolean
  token: string
  tokenParsed: Record<string, unknown> | undefined
  idTokenParsed: Record<string, unknown> | undefined
  onReady: ((authenticated: boolean) => void) | null
  onAuthSuccess: (() => void) | null
  onAuthError: ((error: unknown) => void) | null
  onTokenExpired: (() => void) | null
}

function TestConsumer() {
  const { isAuthenticated, isLoading, isLoggingOut, user, error, login, logout } = useAuth()
  return (
    <div>
      <span data-testid="loading">{String(isLoading)}</span>
      <span data-testid="authenticated">{String(isAuthenticated)}</span>
      <span data-testid="logging-out">{String(isLoggingOut)}</span>
      <span data-testid="user-email">{user?.email || 'none'}</span>
      <span data-testid="user-name">{user?.name || 'none'}</span>
      <span data-testid="error">{error || 'none'}</span>
      <button onClick={() => login()}>Login</button>
      <button onClick={logout}>Logout</button>
    </div>
  )
}

describe('AuthContext', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers({ shouldAdvanceTime: true })
    i18n.addResource('en', 'auth', 'contextErrors.authenticationFailed', 'Authentication failed')
    // Reset mock state
    mockKeycloak.authenticated = true
    mockKeycloak.token = 'mock-token'
    mockKeycloak.tokenParsed = {
      sub: 'user-123',
      email: 'test@example.com',
      preferred_username: 'testuser',
      name: 'Test User',
    }
    mockKeycloak.idTokenParsed = {
      sub: 'user-123',
      email: 'test@example.com',
      preferred_username: 'testuser',
      name: 'Test User',
    }
    mockKeycloak.init.mockResolvedValue(true)
    mockKeycloak.logout.mockResolvedValue(undefined)
    mockKeycloak.updateToken.mockResolvedValue(true)
    // Capture event handlers when they're set
    mockKeycloak.onReady = null
    mockKeycloak.onAuthSuccess = null
    mockKeycloak.onAuthError = null
    mockKeycloak.onTokenExpired = null
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('should provide authentication state after initialization', async () => {
    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    )

    await waitFor(() => {
      expect(screen.getByTestId('loading').textContent).toBe('false')
    })

    expect(screen.getByTestId('authenticated').textContent).toBe('true')
    expect(screen.getByTestId('user-email').textContent).toBe('test@example.com')
  })

  it('should throw error when useAuth is used outside provider', () => {
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

    expect(() => render(<TestConsumer />)).toThrow('useAuth must be used within AuthProvider')

    consoleSpy.mockRestore()
  })

  describe('extractUserFromTokens', () => {
    it('should return null when both tokens are undefined', async () => {
      mockKeycloak.tokenParsed = undefined
      mockKeycloak.idTokenParsed = undefined
      mockKeycloak.init.mockResolvedValue(true)
      mockKeycloak.authenticated = true

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      // Since token is invalid, it should set an error
      expect(screen.getByTestId('error').textContent).toContain('Invalid user data')
    })

    it('should return null when sub claim is missing from both tokens', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
      const claimsWithoutSub = {
        email: 'test@example.com',
        preferred_username: 'testuser',
        name: 'Test User',
      }
      mockKeycloak.tokenParsed = claimsWithoutSub
      mockKeycloak.idTokenParsed = claimsWithoutSub

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      expect(screen.getByTestId('error').textContent).toContain('Invalid user data')
      consoleSpy.mockRestore()
    })

    it('should return null when email claim is missing from both tokens', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
      const claimsWithoutEmail = {
        sub: 'user-123',
        preferred_username: 'testuser',
        name: 'Test User',
      }
      mockKeycloak.tokenParsed = claimsWithoutEmail
      mockKeycloak.idTokenParsed = claimsWithoutEmail

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      expect(screen.getByTestId('error').textContent).toContain('Invalid user data')
      consoleSpy.mockRestore()
    })

    it('should return user with empty name when name is missing', async () => {
      const claimsWithoutName = {
        sub: 'user-123',
        email: 'test@example.com',
        preferred_username: 'testuser',
        // name is missing
      }
      mockKeycloak.tokenParsed = claimsWithoutName
      mockKeycloak.idTokenParsed = claimsWithoutName

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      expect(screen.getByTestId('user-email').textContent).toBe('test@example.com')
      expect(screen.getByTestId('user-name').textContent).toBe('none') // empty string becomes 'none'
    })

    it('should return complete user object with all claims', async () => {
      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      expect(screen.getByTestId('user-email').textContent).toBe('test@example.com')
      expect(screen.getByTestId('user-name').textContent).toBe('Test User')
    })
  })

  describe('AuthProvider initialization', () => {
    it('should handle Keycloak initialization timeout', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

      // Mock init to never resolve (simulating timeout)
      mockKeycloak.init.mockImplementation(() => new Promise(() => {}))

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      // Fast-forward past the timeout (15s)
      await act(async () => {
        vi.advanceTimersByTime(16000)
      })

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      expect(screen.getByTestId('error').textContent).toContain('not responding')
      consoleSpy.mockRestore()
    })

    it('should set error when initialization fails', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
      mockKeycloak.init.mockRejectedValue(new Error('Network error'))

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      expect(screen.getByTestId('error').textContent).toContain('Failed to initialize')
      expect(screen.getByTestId('authenticated').textContent).toBe('false')
      consoleSpy.mockRestore()
    })

    it('should not extract user when token is invalid', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
      mockKeycloak.tokenParsed = {
        // Missing required fields
        some_field: 'value',
      }
      mockKeycloak.idTokenParsed = {
        // Missing required fields
        some_field: 'value',
      }

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      expect(screen.getByTestId('user-email').textContent).toBe('none')
      expect(screen.getByTestId('error').textContent).toContain('Invalid user data')
      consoleSpy.mockRestore()
    })
  })

  describe('Keycloak events', () => {
    it('should handle onReady with authenticated=false', async () => {
      mockKeycloak.init.mockImplementation(async () => {
        // Trigger onReady with false
        if (mockKeycloak.onReady) {
          mockKeycloak.onReady(false)
        }
        return false
      })

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      expect(screen.getByTestId('authenticated').textContent).toBe('false')
      expect(screen.getByTestId('user-email').textContent).toBe('none')
    })

    it('should handle onReady with authenticated=true', async () => {
      mockKeycloak.init.mockImplementation(async () => {
        // Trigger onReady with true
        if (mockKeycloak.onReady) {
          mockKeycloak.onReady(true)
        }
        return true
      })

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      expect(screen.getByTestId('authenticated').textContent).toBe('true')
      expect(screen.getByTestId('user-email').textContent).toBe('test@example.com')
    })

    it('should handle onAuthSuccess and clear errors', async () => {
      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      // Simulate auth success event
      await act(async () => {
        if (mockKeycloak.onAuthSuccess) {
          mockKeycloak.onAuthSuccess()
        }
      })

      expect(screen.getByTestId('authenticated').textContent).toBe('true')
      expect(screen.getByTestId('error').textContent).toBe('none')
    })

    it('should handle onAuthError event', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      // Simulate auth error event
      await act(async () => {
        if (mockKeycloak.onAuthError) {
          mockKeycloak.onAuthError({ error: 'access_denied' })
        }
      })

      expect(screen.getByTestId('authenticated').textContent).toBe('false')
      expect(screen.getByTestId('user-email').textContent).toBe('none')
      expect(screen.getByTestId('error').textContent).toBe('Authentication failed')
      consoleSpy.mockRestore()
    })

    it('should translate auth context error keys through the auth namespace', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
      i18n.addResource(
        'en',
        'auth',
        'contextErrors.authenticationFailed',
        'Localized authentication failure'
      )

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      await act(async () => {
        if (mockKeycloak.onAuthError) {
          mockKeycloak.onAuthError({ error: 'access_denied' })
        }
      })

      expect(screen.getByTestId('error').textContent).toBe('Localized authentication failure')
      consoleSpy.mockRestore()
    })

    it('should handle onTokenExpired event', async () => {
      const consoleSpy = vi.spyOn(console, 'log').mockImplementation(() => {})

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      // Simulate token expired event
      await act(async () => {
        if (mockKeycloak.onTokenExpired) {
          mockKeycloak.onTokenExpired()
        }
      })

      expect(mockKeycloak.updateToken).toHaveBeenCalledWith(60)
      consoleSpy.mockRestore()
    })

    it('should handle token refresh failure on token expired', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
      mockKeycloak.updateToken.mockRejectedValueOnce(new Error('Refresh failed'))

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      // Simulate token expired event with failed refresh
      await act(async () => {
        if (mockKeycloak.onTokenExpired) {
          mockKeycloak.onTokenExpired()
        }
      })

      await waitFor(() => {
        expect(screen.getByTestId('authenticated').textContent).toBe('false')
      })

      expect(screen.getByTestId('error').textContent).toBe('Session expired')
      consoleSpy.mockRestore()
    })

    it('should set up proactive token refresh interval', async () => {
      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      // updateToken should have been called during init
      const initialCalls = mockKeycloak.updateToken.mock.calls.length

      // Fast-forward 30 seconds to trigger the interval
      await act(async () => {
        vi.advanceTimersByTime(30000)
      })

      // Should have called updateToken again
      expect(mockKeycloak.updateToken.mock.calls.length).toBeGreaterThan(initialCalls)
    })
  })

  describe('Login/Logout', () => {
    it('should clear error state before login', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

      // First, set an error state
      mockKeycloak.init.mockRejectedValueOnce(new Error('Init failed'))

      const { rerender } = render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      expect(screen.getByTestId('error').textContent).toContain('Failed to initialize')

      // Now reset mocks and rerender with working init
      mockKeycloak.init.mockResolvedValue(true)

      // Click login - this should clear the error
      rerender(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      await user.click(screen.getByRole('button', { name: /login/i }))

      // login should be called
      expect(mockKeycloak.login).toHaveBeenCalled()
      consoleSpy.mockRestore()
    })

    it('shows a localized error when Keycloak rejects login', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
      mockKeycloak.login.mockRejectedValueOnce(new Error('Redirect unavailable'))

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      await user.click(screen.getByRole('button', { name: /login/i }))

      await waitFor(() => {
        expect(screen.getByTestId('error').textContent).toBe('Authentication failed')
      })
      consoleSpy.mockRestore()
    })

    it('should clear refresh interval on logout', async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      await user.click(screen.getByRole('button', { name: /logout/i }))

      await waitFor(() => expect(mockKeycloak.logout).toHaveBeenCalled())
      expect(screen.getByTestId('authenticated').textContent).toBe('false')
      expect(screen.getByTestId('logging-out').textContent).toBe('true')
      expect(screen.getByTestId('user-email').textContent).toBe('none')
    })

    it('binds the recording store to the signed-in user on init', async () => {
      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })
      expect(claimOutbox).toHaveBeenCalledWith('user-123')
    })

    it('wipes the recording store before handing off to Keycloak logout', async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
      const order: string[] = []
      vi.mocked(wipeOutbox).mockImplementationOnce(async () => {
        order.push('wipe')
      })
      mockKeycloak.logout.mockImplementationOnce(async () => {
        order.push('logout')
      })

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )
      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      await user.click(screen.getByRole('button', { name: /logout/i }))

      await waitFor(() => expect(order).toEqual(['wipe', 'logout']))
    })

    it('still signs out when wiping the recording store fails', async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
      vi.mocked(wipeOutbox).mockRejectedValueOnce(new Error('IndexedDB unavailable'))

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )
      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      await user.click(screen.getByRole('button', { name: /logout/i }))

      await waitFor(() => expect(mockKeycloak.logout).toHaveBeenCalled())
    })

    it('should handle logout network failure gracefully', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

      // Mock window.location
      const originalLocation = window.location
      const mockLocation = { ...window.location, href: '', origin: 'http://localhost' }
      Object.defineProperty(window, 'location', {
        value: mockLocation,
        writable: true,
      })

      mockKeycloak.logout.mockRejectedValue(new Error('Network error'))

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      await user.click(screen.getByRole('button', { name: /logout/i }))

      // State should still be cleared
      expect(screen.getByTestId('authenticated').textContent).toBe('false')
      expect(screen.getByTestId('logging-out').textContent).toBe('true')
      expect(screen.getByTestId('user-email').textContent).toBe('none')

      // Should redirect to origin
      await waitFor(() => expect(mockLocation.href).toBe('http://localhost'))

      // Restore window.location
      Object.defineProperty(window, 'location', {
        value: originalLocation,
        writable: true,
      })
      consoleSpy.mockRestore()
    })
  })

  describe('updateToken', () => {
    it('should call keycloak updateToken with default min validity', async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

      function TestConsumerWithUpdate() {
        const { updateToken, isLoading } = useAuth()
        return (
          <div>
            <span data-testid="loading">{String(isLoading)}</span>
            <button onClick={() => updateToken()}>Update Token</button>
          </div>
        )
      }

      render(
        <AuthProvider>
          <TestConsumerWithUpdate />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      await user.click(screen.getByRole('button', { name: /update token/i }))

      expect(mockKeycloak.updateToken).toHaveBeenCalledWith(30)
    })

    it('should call keycloak updateToken with custom min validity', async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

      function TestConsumerWithCustomUpdate() {
        const { updateToken, isLoading } = useAuth()
        return (
          <div>
            <span data-testid="loading">{String(isLoading)}</span>
            <button onClick={() => updateToken(60)}>Update Token Custom</button>
          </div>
        )
      }

      render(
        <AuthProvider>
          <TestConsumerWithCustomUpdate />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      await user.click(screen.getByRole('button', { name: /update token custom/i }))

      expect(mockKeycloak.updateToken).toHaveBeenCalledWith(60)
    })
  })

  describe('context value', () => {
    it('should provide token from keycloak', async () => {
      function TestConsumerWithToken() {
        const { token, isLoading } = useAuth()
        return (
          <div>
            <span data-testid="loading">{String(isLoading)}</span>
            <span data-testid="token">{token || 'none'}</span>
          </div>
        )
      }

      render(
        <AuthProvider>
          <TestConsumerWithToken />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('loading').textContent).toBe('false')
      })

      expect(screen.getByTestId('token').textContent).toBe('mock-token')
    })
  })
})
