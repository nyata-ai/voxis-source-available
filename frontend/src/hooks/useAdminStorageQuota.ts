import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'

export interface AdminStorageQuota {
  enabled: boolean
  default_limit_bytes: number
  warning_threshold_percent: number
  backend: string
  applies: boolean
  updated_at?: string
  updated_by?: string
}

export interface UpdateAdminStorageQuotaRequest {
  enabled: boolean
  default_limit_bytes: number
}

export const adminStorageQuotaKeys = {
  all: ['admin', 'storage-quota'] as const,
  policy: () => [...adminStorageQuotaKeys.all, 'policy'] as const,
}

export function useAdminStorageQuota() {
  return useQuery({
    queryKey: adminStorageQuotaKeys.policy(),
    queryFn: () => apiClient.get<AdminStorageQuota>('/admin/storage-quota'),
    staleTime: 60_000,
  })
}

export function useUpdateAdminStorageQuota() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: UpdateAdminStorageQuotaRequest) =>
      apiClient.put<AdminStorageQuota>('/admin/storage-quota', req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: adminStorageQuotaKeys.policy() })
    },
  })
}
