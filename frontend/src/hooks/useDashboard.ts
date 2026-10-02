import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'
import type { DashboardStats } from '@/types/dashboard'

export const dashboardKeys = {
  all: ['dashboard'] as const,
  stats: () => [...dashboardKeys.all, 'stats'] as const,
}

export function useDashboardStats() {
  return useQuery({
    queryKey: dashboardKeys.stats(),
    queryFn: () => apiClient.get<DashboardStats>('/dashboard/stats'),
    staleTime: 30_000,
    refetchInterval: 60_000,
  })
}
