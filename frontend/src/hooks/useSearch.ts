import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'
import type { SearchParams, SearchResponse } from '@/types/search'

export const searchKeys = {
  all: ['search'] as const,
  unified: (params: Omit<SearchParams, 'enabled'>) =>
    [...searchKeys.all, 'unified', params] as const,
}

export function useUnifiedSearch(params: SearchParams = {}) {
  const { search = '', limit = 20, offset = 0, transcriptCursor, enabled = true } = params

  const queryParams = new URLSearchParams()
  queryParams.set('search', search)
  queryParams.set('limit', String(limit))
  queryParams.set('offset', String(offset))
  if (transcriptCursor) queryParams.set('transcript_cursor', transcriptCursor)

  return useQuery({
    queryKey: searchKeys.unified({ search, limit, offset, transcriptCursor }),
    queryFn: () => apiClient.get<SearchResponse>(`/search?${queryParams}`),
    staleTime: 30_000,
    enabled: enabled && search.trim().length > 0,
  })
}
