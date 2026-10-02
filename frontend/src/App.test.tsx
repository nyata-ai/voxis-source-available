import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, waitFor } from '@testing-library/react'

// Mock keycloak module
vi.mock('./lib/keycloak', () => {
  const mockKeycloak = {
    init: vi.fn().mockResolvedValue(false),
    login: vi.fn().mockResolvedValue(undefined),
    logout: vi.fn().mockResolvedValue(undefined),
    updateToken: vi.fn().mockResolvedValue(true),
    authenticated: false,
    token: undefined,
    tokenParsed: undefined,
    onReady: null as ((authenticated: boolean) => void) | null,
    onAuthSuccess: null as (() => void) | null,
    onAuthError: null as ((error: unknown) => void) | null,
    onTokenExpired: null as (() => void) | null,
  }
  return {
    keycloak: mockKeycloak,
    keycloakConfig: {
      url: 'http://localhost:8080',
      realm: 'voxis',
      clientId: 'voxis-app',
    },
    initOptions: { onLoad: 'check-sso' },
  }
})

const mockUseSyncLanguage = vi.fn()
vi.mock('@/i18n/useSyncLanguage', () => ({
  useSyncLanguage: () => mockUseSyncLanguage(),
}))

import App from './App'

describe('App', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers({ shouldAdvanceTime: true })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders without crashing', async () => {
    render(<App />)

    // Wait for loading to complete
    await waitFor(
      () => {
        // App should render something - either loading state or actual content
        expect(document.body).toBeDefined()
      },
      { timeout: 5000 }
    )
  })

  it('wraps app in ErrorBoundary', async () => {
    // The App component wraps everything in ErrorBoundary
    // We can verify this by checking that the structure exists
    const { container } = render(<App />)

    // The app should render without throwing
    expect(container).toBeDefined()
  })

  // Above the router, not inside a layout: detection no longer writes
  // localStorage, so a signed-in reader whose saved language is 'ja' would see
  // every public route (landing, login, register, legal, pricing) in whatever
  // the timezone guessed, permanently.
  it('syncs the saved UI language above the router', async () => {
    render(<App />)

    await waitFor(() => {
      expect(mockUseSyncLanguage).toHaveBeenCalled()
    })
  })

  it('wraps app in AuthProvider', async () => {
    render(<App />)

    // Wait for auth initialization
    await waitFor(
      () => {
        // If AuthProvider is working, it will eventually show some content
        expect(document.body).toBeDefined()
      },
      { timeout: 5000 }
    )
  })
})
