import { useQuery, useMutation, useQueryClient, type QueryKey } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'
import type {
  SummaryItem,
  SummaryDetail,
  SummaryListResponse,
  CreateSummaryRequest,
} from '@/types/summary'
import type { SummaryProfile } from '@/types/settings'

export interface RegenerateSummaryInput {
  id: string
  summaryProfile?: SummaryProfile
}

export const summaryKeys = {
  all: ['summaries'] as const,
  list: ({
    limit = 20,
    offset = 0,
    search = '',
  }: { limit?: number; offset?: number; search?: string } = {}) =>
    [...summaryKeys.all, 'list', { limit, offset, search }] as const,
  detail: (id: string) => [...summaryKeys.all, 'detail', id] as const,
  byTranscription: (transcriptionId: string) =>
    [...summaryKeys.all, 'byTranscription', transcriptionId] as const,
}

export function useSummaryList(
  limit = 20,
  offset = 0,
  search = '',
  options: { enabled?: boolean } = {}
) {
  const { enabled = true } = options
  return useQuery({
    queryKey: summaryKeys.list({ limit, offset, search }),
    queryFn: () =>
      apiClient.get<SummaryListResponse>(
        `/summaries?limit=${limit}&offset=${offset}${search ? `&search=${encodeURIComponent(search)}` : ''}`
      ),
    staleTime: 30_000,
    enabled,
  })
}

export function useSummaryDetail(id: string) {
  return useQuery({
    queryKey: summaryKeys.detail(id),
    queryFn: () => apiClient.get<SummaryDetail>(`/summaries/${id}`),
    enabled: !!id,
    // Radix unmounts a collapsed accordion section, so without a staleTime
    // every reopen of a section refires this query (matches useSummaryList).
    staleTime: 30_000,
    refetchInterval: (query) => {
      const status = query.state.data?.status
      if (status === 'pending') {
        return 5_000
      }
      return false
    },
  })
}

export function useSummariesByTranscription(transcriptionId: string) {
  return useQuery({
    queryKey: summaryKeys.byTranscription(transcriptionId),
    queryFn: () => apiClient.get<SummaryItem[]>(`/transcriptions/${transcriptionId}/summaries`),
    enabled: !!transcriptionId,
    staleTime: 30_000,
    refetchInterval: (query) => {
      const summaries = query.state.data
      if (summaries?.some((s) => s.status === 'pending')) {
        return 5_000
      }
      return false
    },
  })
}

/**
 * @param extraKeys Query keys to invalidate alongside the summary caches,
 *   awaited with them. Only a hook-level `onSuccess` holds the mutation open —
 *   TanStack runs mutate-level callbacks after the status has already settled —
 *   so a reader whose summary rows live under some other key (the URL session
 *   embeds them in its detail response) must pass that key here rather than
 *   invalidating it at the call site.
 */
export function useCreateSummary(extraKeys: readonly QueryKey[] = []) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateSummaryRequest) => apiClient.post<SummaryItem>('/summaries', data),
    // Returned, not fired and forgotten: the mutation stays pending until the
    // refetches land, so the button that started this billable job cannot
    // re-enable for the round-trip between the 202 and the list that proves the
    // row exists. A second click in that window pays for the same job twice.
    onSuccess: (_data, variables) =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: summaryKeys.all }),
        queryClient.invalidateQueries({
          queryKey: summaryKeys.byTranscription(variables.transcription_id),
        }),
        ...extraKeys.map((queryKey) => queryClient.invalidateQueries({ queryKey })),
      ]),
  })
}

export interface GenerateAllSummariesInput {
  transcriptionId: string
  summaryProfile?: SummaryProfile
}

/**
 * Fills every generatable summary type for one transcription in a single
 * idempotent round-trip. The endpoint stopped being url-only, so this is the
 * shared "Generate all" path for media-backed sessions too; the URL reader has
 * its own copy because it invalidates a different cache key (its detail
 * response embeds the summaries).
 */
export function useGenerateAllSummariesForTranscription() {
  const queryClient = useQueryClient()
  return useMutation({
    // The 202 body (content-less summary collection) is discarded; the list
    // refetch below is the source of truth.
    mutationFn: ({ transcriptionId, summaryProfile }: GenerateAllSummariesInput) =>
      summaryProfile
        ? apiClient.post<SummaryItem[]>(`/transcriptions/${transcriptionId}/summaries`, {
            summary_profile: summaryProfile,
          })
        : apiClient.post<SummaryItem[]>(`/transcriptions/${transcriptionId}/summaries`),
    // Returned so the mutation stays pending through the refetch: `Generate all`
    // is disabled on `busy || !hasGeneratable`, and both halves read false in
    // the window between the 202 and the refreshed list — one extra click there
    // buys a second eight-token fan-out.
    onSuccess: (_data, variables) =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: summaryKeys.all }),
        queryClient.invalidateQueries({
          queryKey: summaryKeys.byTranscription(variables.transcriptionId),
        }),
      ]),
  })
}

export function useDeleteSummary() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiClient.delete(`/summaries/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: summaryKeys.all })
    },
  })
}

/** `extraKeys` as in {@link useCreateSummary}. */
export function useRegenerateSummary(extraKeys: readonly QueryKey[] = []) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, summaryProfile }: RegenerateSummaryInput) =>
      summaryProfile
        ? apiClient.post<SummaryItem>(`/summaries/${id}/regenerate`, {
            summary_profile: summaryProfile,
          })
        : apiClient.post<SummaryItem>(`/summaries/${id}/regenerate`),
    // `regeneratingId` disables every Regenerate button and clears on settle.
    // Waiting for the re-queued row to reach the cache prevents a rapid
    // duplicate summary request.
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: summaryKeys.all }),
        ...extraKeys.map((queryKey) => queryClient.invalidateQueries({ queryKey })),
      ]),
  })
}
