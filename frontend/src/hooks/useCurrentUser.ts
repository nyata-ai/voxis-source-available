import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'

// Response types matching backend handler.MeResponse
export interface Organization {
  id: string
  name: string
  tier: string
}

export interface CurrentUser {
  id: string
  email: string
  name: string
  organization?: Organization
}

export const userKeys = {
  all: ['user'] as const,
  current: () => [...userKeys.all, 'current'] as const,
}

export function useCurrentUser() {
  return useQuery({
    queryKey: userKeys.current(),
    queryFn: () => apiClient.get<CurrentUser>('/me'),
    // User data is relatively stable, refetch less frequently
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}
