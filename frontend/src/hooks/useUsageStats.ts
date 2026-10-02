import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'
import type { UsageStats } from '@/types/usage'

export const usageKeys = {
  all: ['usage'] as const,
  stats: () => [...usageKeys.all, 'stats'] as const,
}

export function useUsageStats() {
  return useQuery({
    queryKey: usageKeys.stats(),
    queryFn: () => apiClient.get<UsageStats>('/usage/stats'),
    staleTime: 60_000,
  })
}
