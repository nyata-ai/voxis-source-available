import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient, ApiError, resolveApiUrl } from '@/lib/api-client'
import type { MediaItem, MediaListResponse, MediaSearchParams } from '@/types/media'
import { usageKeys } from '@/hooks/useUsageStats'

export const mediaKeys = {
  all: ['media'] as const,
  detail: (id: string) => [...mediaKeys.all, 'detail', id] as const,
  search: (params: MediaSearchParams) => [...mediaKeys.all, 'search', params] as const,
}

export function useSearchMedia(params: MediaSearchParams = {}) {
  const { search = '', status = '', limit = 20, offset = 0, enabled = true } = params

  const queryParams = new URLSearchParams()
  queryParams.set('limit', String(limit))
  queryParams.set('offset', String(offset))
  if (search) queryParams.set('search', search)
  if (status) queryParams.set('status', status)

  return useQuery({
    queryKey: mediaKeys.search({ search, status, limit, offset }),
    queryFn: () => apiClient.get<MediaListResponse>(`/media?${queryParams}`),
    staleTime: 30_000,
    enabled,
  })
}

const MEDIA_POLL_INTERVAL_MS = 5_000

/**
 * How long to wait before re-reading a media row, or `false` once nothing about
 * it is still being decided.
 *
 * `status` covers the encrypt-and-store phase, but a row reaches `ready` with
 * the malware scan still running, and that verdict is what decides whether
 * playback, download and transcription may be offered. Stopping at `ready` left
 * a freshly uploaded file frozen in `scan_pending` until a manual reload.
 */
export function mediaPollInterval(media: MediaItem | undefined): number | false {
  if (!media) return false
  if (media.status === 'pending' || media.status === 'encrypting') return MEDIA_POLL_INTERVAL_MS
  if (media.scan_status === 'scan_pending') return MEDIA_POLL_INTERVAL_MS
  return false
}

export function useMediaDetail(id: string) {
  return useQuery({
    queryKey: mediaKeys.detail(id),
    queryFn: () => apiClient.get<MediaItem>(`/media/${id}`),
    enabled: !!id,
    refetchInterval: (query) => mediaPollInterval(query.state.data),
  })
}

export function useUploadMedia() {
  const queryClient = useQueryClient()

  type UploadPayload = {
    formData: FormData
    onProgress?: (pct: number) => void
  }

  return useMutation({
    mutationFn: ({ formData, onProgress }: UploadPayload) =>
      apiClient.upload<MediaItem>('/media/upload', formData, onProgress),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: mediaKeys.all })
      queryClient.invalidateQueries({ queryKey: usageKeys.stats() })
    },
  })
}

export function useUpdateMedia() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: { title?: string; description?: string } }) =>
      apiClient.patch<MediaItem>(`/media/${id}`, data),
    onSuccess: (updatedMedia) => {
      queryClient.setQueryData(mediaKeys.detail(updatedMedia.id), updatedMedia)
      queryClient.invalidateQueries({ queryKey: mediaKeys.all })
      queryClient.invalidateQueries({ queryKey: ['transcriptions'] })
    },
  })
}

export function useDeleteMedia() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (id: string) => apiClient.delete(`/media/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: mediaKeys.all })
      queryClient.invalidateQueries({ queryKey: usageKeys.stats() })
      // The server cascades the delete through transcriptions and summaries,
      // so any cached transcription list still names a row that is gone.
      queryClient.invalidateQueries({ queryKey: ['transcriptions'] })
    },
  })
}

export function useMediaStreamUrl(id: string) {
  return useQuery({
    queryKey: [...mediaKeys.detail(id), 'stream-url'],
    queryFn: async () => {
      const response = await apiClient.getStreamUrl(id)
      return { ...response, url: resolveApiUrl(response.url) }
    },
    enabled: !!id,
    // Stream URLs are signed/short-lived; avoid serving cached stale tokens.
    staleTime: 0,
    gcTime: 60_000,
    // ...but not on focus. `staleTime: 0` plus the client's global
    // refetchOnWindowFocus would mint a NEW signed URL every time the reader
    // alt-tabs back, and a changed `src` resets the <audio> element to 0. The
    // player only ever needs one URL per mount; a genuinely expired one comes
    // back through the retry path below.
    refetchOnWindowFocus: false,
    retry: (count, error) => {
      // Don't retry scan-related errors (409 pending, 403 failed).
      if (error instanceof ApiError && (error.status === 409 || error.status === 403)) {
        return false
      }
      return count < 1
    },
  })
}
