import { useQuery } from '@tanstack/react-query'
import { apiClient, ApiError } from '@/lib/api-client'
import { useAuth } from '@/contexts/AuthContext'

export interface AdminMe {
  admin: boolean
}

export const adminKeys = {
  all: ['admin'] as const,
  me: (subject: string) => [...adminKeys.all, 'me', subject] as const,
}

export function useAdmin() {
  const { user } = useAuth()
  const subject = user?.id ?? 'anonymous'

  return useQuery({
    queryKey: adminKeys.me(subject),
    queryFn: () => apiClient.get<AdminMe>('/admin/me'),
    enabled: user !== null,
    retry: (failureCount, error) => {
      if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
        return false
      }
      return failureCount < 3
    },
    staleTime: 0,
    refetchOnMount: 'always',
  })
}
