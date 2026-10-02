import { useQuery, type QueryClient } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'
import type { ActivityItem, ActivityResponse } from '@/types/activity'

export const activityKeys = {
  all: ['activity'] as const,
}

const ACTIVE_POLL_MS = 3_000
const IDLE_HEARTBEAT_MS = 30_000

// Poll fast while anything is in progress; keep a slow idle heartbeat so work
// started in another tab/device surfaces without a manual refetch.
export function getActivityRefetchInterval(items?: ActivityItem[]): number {
  if (!items) return IDLE_HEARTBEAT_MS
  return items.some((i) => i.status === 'in_progress') ? ACTIVE_POLL_MS : IDLE_HEARTBEAT_MS
}

export function optimisticRecordingItem(sessionId: string, startedAt: string): ActivityItem {
  return {
    kind: 'recording',
    ref_id: sessionId,
    stage: 'stitching',
    status: 'in_progress',
    started_at: startedAt,
  }
}

export function optimisticTranscriptionItem(id: string, startedAt: string): ActivityItem {
  const basePath = '/transcriptions'
  return {
    kind: 'transcription',
    ref_id: id,
    stage: 'transcribing',
    status: 'in_progress',
    link: `${basePath}/${id}`,
    started_at: startedAt,
  }
}

export function seedActivity(queryClient: QueryClient, item: ActivityItem) {
  queryClient.setQueryData<ActivityResponse>(activityKeys.all, (old) => {
    const items = old?.items ?? []
    if (items.some((i) => i.ref_id === item.ref_id)) return old
    return { items: [item, ...items] }
  })
}

export function useActivity() {
  return useQuery({
    queryKey: activityKeys.all,
    queryFn: () => apiClient.get<ActivityResponse>('/activity'),
    staleTime: 0,
    refetchInterval: (query) => getActivityRefetchInterval(query.state.data?.items),
    placeholderData: (prev) => prev,
  })
}
