import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, apiClient } from '@/lib/api-client'
import { useAuth } from '@/contexts/AuthContext'
import type { UserPreferences } from '@/types/settings'

export interface PasswordSecurityRequest {
  new_password: string
  confirm_password: string
}

export interface SecurityMessageResponse {
  message: string
}

export interface MfaCredential {
  id: string
  type: string
  user_label: string
  created_at?: string
}

export interface MfaSecurityResponse {
  enabled: boolean
  credentials: MfaCredential[]
}

interface ErrorBody {
  error?: string
}

export function isRecentAuthRequiredError(error: unknown): boolean {
  if (!(error instanceof ApiError) || error.status !== 403) {
    return false
  }
  const data = error.data as ErrorBody | null | undefined
  return data?.error === 'recent_auth_required'
}

export const settingsKeys = {
  all: ['settings'] as const,
  preferences: () => [...settingsKeys.all, 'preferences'] as const,
  security: () => [...settingsKeys.all, 'security'] as const,
  mfa: () => [...settingsKeys.security(), 'mfa'] as const,
}

export function usePreferences() {
  // Gated because useSyncLanguage() calls this from above the router, so it is
  // live on the public routes too. Every other caller sits behind ProtectedRoute
  // and is unaffected: there isAuthenticated is already true.
  const { isAuthenticated } = useAuth()
  return useQuery({
    queryKey: settingsKeys.preferences(),
    queryFn: () => apiClient.get<UserPreferences>('/settings/preferences'),
    staleTime: 5 * 60_000, // 5 minutes
    enabled: isAuthenticated,
  })
}

export function useUpdatePreferences() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (prefs: Partial<UserPreferences>) =>
      apiClient.put<UserPreferences>('/settings/preferences', prefs),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.preferences() })
    },
  })
}

export function useUpdatePasswordSecurity() {
  return useMutation({
    mutationFn: (req: PasswordSecurityRequest) =>
      apiClient.put<SecurityMessageResponse>('/settings/security/password', req),
  })
}

export function useMfaSecurity(enabled = true) {
  return useQuery({
    queryKey: settingsKeys.mfa(),
    queryFn: () => apiClient.get<MfaSecurityResponse>('/settings/security/mfa'),
    enabled,
  })
}

export function useDeleteMfaCredential() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (credentialId: string) =>
      apiClient.delete<SecurityMessageResponse>(`/settings/security/mfa/${encodeURIComponent(credentialId)}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.mfa() })
    },
  })
}
