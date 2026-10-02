import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'
import {
  useDeleteMfaCredential,
  useMfaSecurity,
  usePreferences,
  useUpdatePasswordSecurity,
  useUpdatePreferences,
  settingsKeys,
} from './useSettings'
import { apiClient } from '@/lib/api-client'

vi.mock('@/lib/api-client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}))

const auth = { isAuthenticated: true }
vi.mock('@/contexts/AuthContext', () => ({
  useAuth: () => auth,
}))

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
      mutations: {
        retry: false,
      },
    },
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children)
  }
}

const mockPreferences = {
  theme: 'system' as const,
  default_languages: ['id'],
  default_diarization: true,
  default_summary_type: 'general' as const,
  default_export_format: 'pdf' as const,
  playback_speed: 1.0,
  high_stakes_summaries: false,
}

describe('settingsKeys', () => {
  it('should return correct all key', () => {
    expect(settingsKeys.all).toEqual(['settings'])
  })

  it('should return correct preferences key', () => {
    expect(settingsKeys.preferences()).toEqual(['settings', 'preferences'])
  })

  it('should return correct security keys', () => {
    expect(settingsKeys.security()).toEqual(['settings', 'security'])
    expect(settingsKeys.mfa()).toEqual(['settings', 'security', 'mfa'])
  })

})

describe('usePreferences', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    auth.isAuthenticated = true
  })

  afterEach(() => {
    vi.resetAllMocks()
    auth.isAuthenticated = true
  })

  // The hook is now mounted above the router, so it runs on the public landing,
  // login and legal routes too. Without this gate every anonymous visit would
  // fire an authenticated request and take a 401.
  it('does not fetch for an anonymous visitor', async () => {
    auth.isAuthenticated = false

    const { result } = renderHook(() => usePreferences(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.fetchStatus).toBe('idle')
    })
    expect(apiClient.get).not.toHaveBeenCalled()
    expect(result.current.data).toBeUndefined()
  })

  it('should fetch preferences successfully', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce(mockPreferences)

    const { result } = renderHook(() => usePreferences(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data).toEqual(mockPreferences)
    expect(apiClient.get).toHaveBeenCalledWith('/settings/preferences')
  })

  it('should surface network error', async () => {
    const error = new Error('Network error')
    vi.mocked(apiClient.get).mockRejectedValueOnce(error)

    const { result } = renderHook(() => usePreferences(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(result.current.error).toBe(error)
  })
})

describe('useUpdatePreferences', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should update preferences with PUT', async () => {
    const updatedPrefs = { ...mockPreferences, theme: 'dark' as const }
    vi.mocked(apiClient.put).mockResolvedValueOnce(updatedPrefs)

    const { result } = renderHook(() => useUpdatePreferences(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ theme: 'dark' })
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.put).toHaveBeenCalledWith('/settings/preferences', { theme: 'dark' })
    expect(result.current.data).toEqual(updatedPrefs)
  })

  it('should surface 400 validation error', async () => {
    const error = new Error('Bad Request')
    vi.mocked(apiClient.put).mockRejectedValueOnce(error)

    const { result } = renderHook(() => useUpdatePreferences(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ playback_speed: -1 })
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(result.current.error).toBe(error)
  })

  it('should invalidate preferences query on success', async () => {
    vi.mocked(apiClient.get).mockResolvedValue(mockPreferences)
    vi.mocked(apiClient.put).mockResolvedValue({ ...mockPreferences, theme: 'dark' as const })

    const wrapper = createWrapper()

    // First fetch preferences to populate the cache
    const { result: queryResult } = renderHook(() => usePreferences(), { wrapper })
    await waitFor(() => expect(queryResult.current.isSuccess).toBe(true))

    // Then mutate
    const { result: mutationResult } = renderHook(() => useUpdatePreferences(), { wrapper })

    await act(async () => {
      mutationResult.current.mutate({ theme: 'dark' })
    })

    await waitFor(() => expect(mutationResult.current.isSuccess).toBe(true))

    // apiClient.get should have been called again due to query invalidation
    expect(apiClient.get).toHaveBeenCalledTimes(2)
  })
})

describe('security settings hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  it('should update password with PUT', async () => {
    vi.mocked(apiClient.put).mockResolvedValueOnce({ message: 'password updated' })

    const { result } = renderHook(() => useUpdatePasswordSecurity(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        new_password: 'ValidPass123!',
        confirm_password: 'ValidPass123!',
      })
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(apiClient.put).toHaveBeenCalledWith('/settings/security/password', {
      new_password: 'ValidPass123!',
      confirm_password: 'ValidPass123!',
    })
  })

  it('should fetch MFA settings', async () => {
    const mfa = {
      enabled: true,
      credentials: [{ id: 'otp-1', type: 'otp', user_label: 'Authenticator app' }],
    }
    vi.mocked(apiClient.get).mockResolvedValueOnce(mfa)

    const { result } = renderHook(() => useMfaSecurity(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data).toEqual(mfa)
    expect(apiClient.get).toHaveBeenCalledWith('/settings/security/mfa')
  })

  it('should delete MFA credential and invalidate MFA query', async () => {
    const mfa = {
      enabled: true,
      credentials: [{ id: 'otp-1', type: 'otp', user_label: 'Authenticator app' }],
    }
    vi.mocked(apiClient.get).mockResolvedValue(mfa)
    vi.mocked(apiClient.delete).mockResolvedValueOnce({ message: 'mfa credential removed' })
    const wrapper = createWrapper()

    const { result: queryResult } = renderHook(() => useMfaSecurity(), { wrapper })
    await waitFor(() => expect(queryResult.current.isSuccess).toBe(true))

    const { result: mutationResult } = renderHook(() => useDeleteMfaCredential(), { wrapper })
    await act(async () => {
      mutationResult.current.mutate('otp-1')
    })

    await waitFor(() => expect(mutationResult.current.isSuccess).toBe(true))

    expect(apiClient.delete).toHaveBeenCalledWith('/settings/security/mfa/otp-1')
    expect(apiClient.get).toHaveBeenCalledTimes(2)
  })
})
