import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'
import { mediaKeys } from '@/hooks/useMedia'
import { activityKeys, optimisticTranscriptionItem, seedActivity } from './useActivity'
import type {
  TranscriptionItem,
  TranscriptionDetail,
  TranscriptionListResponse,
  CreateTranscriptionRequest,
} from '@/types/transcription'

export const transcriptionKeys = {
  all: ['transcriptions'] as const,
  list: ({
    limit = 20,
    offset = 0,
    search = '',
  }: { limit?: number; offset?: number; search?: string } = {}) =>
    [...transcriptionKeys.all, 'list', { limit, offset, search }] as const,
  detail: (id: string) => [...transcriptionKeys.all, 'detail', id] as const,
}

// Mirrors getURLTranscriptionListRefetchInterval — keeps in-progress rows live
// (10s while any item is pending/submitted/processing) without polling forever.
export function getTranscriptionListRefetchInterval(items?: TranscriptionItem[]) {
  if (!items) return false
  const hasActive = items.some(
    (i) => i.status === 'pending' || i.status === 'submitted' || i.status === 'processing'
  )
  return hasActive ? 10_000 : false
}

export function useTranscriptionList(
  limit = 20,
  offset = 0,
  search = '',
  options: { enabled?: boolean } = {}
) {
  const { enabled = true } = options
  return useQuery({
    queryKey: transcriptionKeys.list({ limit, offset, search }),
    queryFn: () =>
      apiClient.get<TranscriptionListResponse>(
        `/transcriptions?limit=${limit}&offset=${offset}${search ? `&search=${encodeURIComponent(search)}` : ''}`
      ),
    staleTime: 30_000,
    refetchInterval: (query) => getTranscriptionListRefetchInterval(query.state.data?.items),
    enabled,
  })
}

export function useTranscriptionDetail(id: string) {
  return useQuery({
    queryKey: transcriptionKeys.detail(id),
    queryFn: () => apiClient.get<TranscriptionDetail>(`/transcriptions/${id}`),
    enabled: !!id,
    refetchInterval: (query) => {
      const status = query.state.data?.status
      if (status === 'pending' || status === 'submitted') {
        return 5_000
      }
      return false
    },
  })
}

export function useCreateTranscription() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateTranscriptionRequest) =>
      apiClient.post<TranscriptionItem>('/transcriptions', data),
    onSuccess: (created) => {
      queryClient.invalidateQueries({ queryKey: transcriptionKeys.all })
      queryClient.invalidateQueries({ queryKey: mediaKeys.all })
      seedActivity(queryClient, optimisticTranscriptionItem(created.id, new Date().toISOString()))
      queryClient.invalidateQueries({ queryKey: activityKeys.all })
    },
  })
}

export function useDeleteTranscription() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiClient.delete(`/transcriptions/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: transcriptionKeys.all })
    },
  })
}

export function useUpdateSpeakers(transcriptionId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (speakerMap: Record<string, string>) =>
      apiClient.patch(`/transcriptions/${transcriptionId}/speakers`, { speaker_map: speakerMap }),
    onSuccess: () => {
      // Speaker names are read by the transcription detail route, so refresh its
      // cache after a successful update.
      queryClient.invalidateQueries({ queryKey: transcriptionKeys.detail(transcriptionId) })
    },
  })
}

// Generate AI speaker-name suggestions lazily and idempotently.
export function useGenerateSpeakerSuggestions(transcriptionId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiClient.post(`/transcriptions/${transcriptionId}/speakers/suggestions`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: transcriptionKeys.detail(transcriptionId) })
    },
  })
}

// Dismiss a single speaker suggestion by index. Does not touch speaker_map.
export function useDismissSpeakerSuggestion(transcriptionId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (index: string) =>
      apiClient.delete(`/transcriptions/${transcriptionId}/speakers/suggestions/${index}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: transcriptionKeys.detail(transcriptionId) })
    },
  })
}
