import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'
import type { InterruptedRecordingsResponse } from '@/types/recording'
import { recordingKeys } from './useRecordingSession'

/**
 * Fetch interrupted recording sessions for the current user.
 * Called on Media Library mount and /record page mount.
 */
export function useInterruptedRecording() {
  return useQuery({
    queryKey: recordingKeys.interrupted(),
    queryFn: () =>
      apiClient.get<InterruptedRecordingsResponse>('/recordings/interrupted'),
    staleTime: 30_000,
  })
}
