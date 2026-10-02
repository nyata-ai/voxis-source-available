/* Voxis Source-Available local Keycloak authentication. See the repository licensing files. */

import { useEffect, useState, useCallback, useMemo, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { keycloak, initOptions } from '../lib/keycloak'
import { claimOutbox, wipeOutbox } from '../lib/chunk-outbox'
import { AuthContext, type AuthContextType, type User } from './AuthContext'

// Token refresh timing constants
const TOKEN_REFRESH_MIN_VALIDITY_SECONDS = 60 // Refresh if token expires within this time
const TOKEN_REFRESH_INTERVAL_MS = 30_000 // Check for token refresh every 30s
const KEYCLOAK_INIT_TIMEOUT_MS = 15_000 // Timeout for Keycloak initialization
// Sign-out waits this long for the browser recording store to be wiped. A
// stuck IndexedDB must not trap the user in the app.
const OUTBOX_WIPE_TIMEOUT_MS = 3_000

type AuthErrorKey =
  | 'invalidUserData'
  | 'authenticationFailed'
  | 'sessionExpired'
  | 'serviceNotResponding'
  | 'initFailed'

interface AuthProviderProps {
  children: ReactNode
}

/**
 * Extract user information from Keycloak tokens.
 * Uses ID token as primary source (always has `sub`), falls back to access token.
 * Keycloak 26+ may omit `sub` from access tokens depending on client scope config.
 */
function extractUserFromTokens(
  idTokenParsed: Keycloak.KeycloakTokenParsed | undefined,
  tokenParsed: Keycloak.KeycloakTokenParsed | undefined
): User | null {
  // Prefer ID token (has sub), fall back to access token
  const primary = idTokenParsed || tokenParsed
  if (!primary) return null

  const sub = primary.sub || tokenParsed?.sub
  const email = (primary.email || tokenParsed?.email) as string | undefined
  const username = (primary.preferred_username || tokenParsed?.preferred_username) as
    | string
    | undefined

  if (!sub || !email || !username) {
    if (import.meta.env.DEV) {
      console.error('Missing required claims in token:', { sub, email, username })
    }
    return null
  }

  return {
    id: sub,
    email,
    name: ((primary.name || tokenParsed?.name) as string) || '',
    username,
  }
}

/**
 * Create a promise that rejects after a timeout
 */
function createTimeout(ms: number, message: string): Promise<never> {
  return new Promise((_, reject) => {
    setTimeout(() => reject(new Error(message)), ms)
  })
}

export function AuthProvider({ children }: AuthProviderProps) {
  const { t } = useTranslation('auth')
  const [isLoading, setIsLoading] = useState(true)
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [isLoggingOut, setIsLoggingOut] = useState(false)
  const [user, setUser] = useState<User | null>(null)
  const [errorKey, setErrorKey] = useState<AuthErrorKey | null>(null)
  const initStarted = useRef(false)
  const refreshIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const error = useMemo(() => (errorKey ? t(`contextErrors.${errorKey}`) : null), [errorKey, t])

  // Cleanup function to clear refresh interval
  const clearRefreshInterval = useCallback(() => {
    if (refreshIntervalRef.current) {
      clearInterval(refreshIntervalRef.current)
      refreshIntervalRef.current = null
    }
  }, [])

  useEffect(() => {
    // Prevent double initialization in React StrictMode
    if (initStarted.current) return
    initStarted.current = true

    const initKeycloak = async () => {
      try {
        // Set up event handlers before init
        keycloak.onReady = (authenticated) => {
          setIsAuthenticated(authenticated ?? false)
          if (authenticated) {
            const extractedUser = extractUserFromTokens(
              keycloak.idTokenParsed,
              keycloak.tokenParsed
            )
            if (extractedUser) {
              setUser(extractedUser)
            } else {
              setErrorKey('invalidUserData')
            }
          }
        }

        keycloak.onAuthSuccess = () => {
          setIsAuthenticated(true)
          setErrorKey(null)
          const extractedUser = extractUserFromTokens(keycloak.idTokenParsed, keycloak.tokenParsed)
          if (extractedUser) {
            setUser(extractedUser)
          }
        }

        keycloak.onAuthError = (errorData) => {
          if (import.meta.env.DEV) {
            console.error('Auth error:', errorData)
          }
          clearRefreshInterval()
          setIsAuthenticated(false)
          setUser(null)
          setErrorKey('authenticationFailed')
        }

        keycloak.onTokenExpired = () => {
          if (import.meta.env.DEV) {
            console.log('Token expired, attempting refresh...')
          }
          keycloak.updateToken(TOKEN_REFRESH_MIN_VALIDITY_SECONDS).catch(() => {
            if (import.meta.env.DEV) {
              console.error('Failed to refresh expired token')
            }
            clearRefreshInterval()
            setIsAuthenticated(false)
            setUser(null)
            setErrorKey('sessionExpired')
          })
        }

        // Initialize Keycloak with timeout
        const authenticated = await Promise.race([
          keycloak.init(initOptions),
          createTimeout(KEYCLOAK_INIT_TIMEOUT_MS, 'Keycloak initialization timeout'),
        ])

        setIsAuthenticated(authenticated)

        if (authenticated) {
          const extractedUser = extractUserFromTokens(keycloak.idTokenParsed, keycloak.tokenParsed)
          if (extractedUser) {
            // Before any page renders: the browser recording store must never
            // expose another user's unsent audio. Outbox reads wait for this.
            claimOutbox(extractedUser.id).catch(() => {})
            setUser(extractedUser)
          } else {
            setErrorKey('invalidUserData')
            setIsAuthenticated(false)
            return
          }

          // Set up proactive token refresh
          refreshIntervalRef.current = setInterval(async () => {
            try {
              await keycloak.updateToken(TOKEN_REFRESH_MIN_VALIDITY_SECONDS)
            } catch {
              if (import.meta.env.DEV) {
                console.error('Proactive token refresh failed')
              }
              // Don't clear interval here - let onTokenExpired handle session expiry
            }
          }, TOKEN_REFRESH_INTERVAL_MS)
        }
      } catch (err) {
        if (import.meta.env.DEV) {
          console.error('Keycloak initialization failed:', err)
        }
        setIsAuthenticated(false)
        setErrorKey(
          err instanceof Error && err.message.includes('timeout')
            ? 'serviceNotResponding'
            : 'initFailed'
        )
      } finally {
        setIsLoading(false)
      }
    }

    initKeycloak()

    return () => {
      clearRefreshInterval()
    }
  }, [clearRefreshInterval])

  const login = useCallback(async (redirectUri?: string) => {
    setErrorKey(null)
    setIsLoggingOut(false)

    try {
      if (redirectUri) {
        // Defense-in-depth: ensure redirect stays on our origin.
        const url = new URL(redirectUri, window.location.origin)
        if (url.origin !== window.location.origin) {
          if (import.meta.env.DEV) {
            console.error('Blocked cross-origin redirect:', redirectUri)
          }
          return
        }
        await keycloak.login({ redirectUri: url.href })
        return
      }

      await keycloak.login()
    } catch (err) {
      if (import.meta.env.DEV) {
        console.error('Keycloak login failed:', err)
      }
      setErrorKey('authenticationFailed')
    }
  }, [])

  const logout = useCallback(async () => {
    clearRefreshInterval()
    setIsLoggingOut(true)
    // Clear local state regardless of logout success
    setUser(null)
    setIsAuthenticated(false)

    // Unsent recording chunks and their key stay in this browser otherwise,
    // readable by the next person who signs in on it. Callers warn first.
    await Promise.race([
      wipeOutbox(),
      createTimeout(OUTBOX_WIPE_TIMEOUT_MS, 'Recording store wipe timeout'),
    ]).catch(() => {
      // Best effort: the next sign-in as a different user wipes it as well.
    })

    try {
      await keycloak.logout({
        redirectUri: window.location.origin,
      })
    } catch (err) {
      // Logout failed (network issue) - local state already cleared
      if (import.meta.env.DEV) {
        console.error('Logout failed:', err)
      }
      // Still redirect to home/login page
      window.location.href = window.location.origin
    }
  }, [clearRefreshInterval])

  const updateToken = useCallback(async (minValidity = 30) => {
    return keycloak.updateToken(minValidity)
  }, [])

  const value: AuthContextType = {
    isAuthenticated,
    isLoading,
    isLoggingOut,
    user,
    token: keycloak.token,
    error,
    login,
    logout,
    updateToken,
  }

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
