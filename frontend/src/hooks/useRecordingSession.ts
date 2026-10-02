import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, apiClient } from '@/lib/api-client'
import { retryOnRateLimit } from '@/lib/api-errors'
import type {
  RecordingSession,
  CreateRecordingRequest,
} from '@/types/recording'
import { activityKeys, optimisticRecordingItem, seedActivity } from './useActivity'
import { mediaKeys } from './useMedia'
import { usageKeys } from './useUsageStats'

export const recordingKeys = {
  all: ['recordings'] as const,
  interrupted: () => [...recordingKeys.all, 'interrupted'] as const,
  detail: (id: string) => [...recordingKeys.all, 'detail', id] as const,
}

/** Create a new recording session. */
export function useCreateRecordingSession() {
  return useMutation({
    mutationFn: (data: CreateRecordingRequest) =>
      apiClient.post<RecordingSession>('/recordings', data),
  })
}

/** Complete a recording session (triggers server-side stitching). */
export function useCompleteRecordingSession() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (sessionId: string) =>
      retryOnRateLimit(() =>
        apiClient.post<RecordingSession>(`/recordings/${sessionId}/complete`)
      ),
    onSuccess: (_data, sessionId) => {
      queryClient.invalidateQueries({ queryKey: mediaKeys.all })
      queryClient.invalidateQueries({ queryKey: usageKeys.stats() })
      seedActivity(queryClient, optimisticRecordingItem(sessionId, new Date().toISOString()))
      queryClient.invalidateQueries({ queryKey: activityKeys.all })
    },
  })
}

/** Send session activity heartbeat. */
export async function sendHeartbeat(sessionId: string): Promise<void> {
  await apiClient.post<void>(`/recordings/${sessionId}/heartbeat`)
}

/** Pause a recording session. */
export function usePauseRecordingSession() {
  return useMutation({
    mutationFn: (sessionId: string) =>
      apiClient.patch<RecordingSession>(`/recordings/${sessionId}/pause`, {}),
  })
}

/** Resume a paused recording session. */
export function useResumeRecordingSession() {
  return useMutation({
    mutationFn: (sessionId: string) =>
      apiClient.patch<RecordingSession>(`/recordings/${sessionId}/resume`, {}),
  })
}

/** Recover an interrupted session (triggers server-side stitching). */
export function useRecoverRecordingSession() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (sessionId: string) =>
      retryOnRateLimit(() =>
        apiClient.post<RecordingSession>(`/recordings/${sessionId}/recover`)
      ),
    onSuccess: (_data, sessionId) => {
      queryClient.invalidateQueries({ queryKey: recordingKeys.interrupted() })
      queryClient.invalidateQueries({ queryKey: usageKeys.stats() })
      seedActivity(queryClient, optimisticRecordingItem(sessionId, new Date().toISOString()))
      queryClient.invalidateQueries({ queryKey: activityKeys.all })
    },
  })
}

/**
 * Fetch the caller's still-active (recording|paused) session, if any.
 * Returns null when the server reports none (404).
 */
export async function fetchActiveRecordingSession(): Promise<RecordingSession | null> {
  try {
    return await apiClient.get<RecordingSession>('/recordings/active')
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) {
      return null
    }
    throw err
  }
}

/**
 * Release an active session (recording|paused → interrupted).
 * Turns a session stranded by a reload/crash into one the recovery dialog can
 * offer immediately, instead of waiting for the 30-minute orphan sweep.
 */
export function useReleaseRecordingSession() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (sessionId: string) =>
      apiClient.post<RecordingSession>(`/recordings/${sessionId}/release`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: recordingKeys.interrupted() })
    },
  })
}

/** Abandon an interrupted session (cleanup). */
export function useAbandonRecordingSession() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (sessionId: string) =>
      retryOnRateLimit(() => apiClient.delete<void>(`/recordings/${sessionId}`)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: recordingKeys.interrupted() })
      queryClient.invalidateQueries({ queryKey: usageKeys.stats() })
    },
  })
}

/**
 * Upload a single chunk via multipart form.
 * Used by the chunk upload hook to flush outbox entries.
 */
export async function uploadChunk(
  sessionId: string,
  seq: number,
  blob: Blob,
): Promise<void> {
  const formData = new FormData()
  formData.append('file', blob)
  formData.append('seq', String(seq))

  await apiClient.upload<void>(`/recordings/${sessionId}/chunks`, formData)
}
