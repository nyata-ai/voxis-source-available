import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'

export interface RecordingRetentionPolicy {
  enabled: boolean
  days: number
  effective_at?: string
  apply_to_existing: boolean
  updated_at?: string
  updated_by?: string
}

export interface RecordingRetentionRequest {
  enabled: boolean
  days: number
  apply_to_existing: boolean
  confirmed_immediate_delete_count?: number
}

export interface RecordingRetentionPreview {
  immediate_delete_count: number
  oldest_completed_at?: string
  prospective_effective_at?: string
}

export const adminRecordingRetentionKeys = {
  all: ['admin', 'recording-retention'] as const,
  policy: () => [...adminRecordingRetentionKeys.all, 'policy'] as const,
}

export function useAdminRecordingRetentionPolicy() {
  return useQuery({
    queryKey: adminRecordingRetentionKeys.policy(),
    queryFn: () => apiClient.get<RecordingRetentionPolicy>('/admin/recording-retention'),
    staleTime: 60_000,
  })
}

export function useAdminRecordingRetentionPreview() {
  return useMutation({
    mutationFn: (req: RecordingRetentionRequest) =>
      apiClient.post<RecordingRetentionPreview>('/admin/recording-retention/preview', req),
  })
}

export function useUpdateAdminRecordingRetentionPolicy() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: RecordingRetentionRequest) =>
      apiClient.put<RecordingRetentionPolicy>('/admin/recording-retention', req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: adminRecordingRetentionKeys.policy() })
    },
  })
}
