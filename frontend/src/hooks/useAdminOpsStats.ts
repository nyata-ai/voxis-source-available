import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'

export interface AdminOpsEntityStats {
  total: number
  by_status: Record<string, number>
}
export interface AdminOpsTranscriptionStats extends AdminOpsEntityStats {
  stale_submitted_over_2h: number
}
export interface AdminOpsSummaryStats extends AdminOpsEntityStats {
  stale_pending_over_30m: number
}
export interface AdminSTTUsageStats {
  submitted_jobs: number
  completed_jobs: number
  failed_jobs: number
  stale_submitted_over_2h: number
  processed_audio_seconds: number
}
export interface AdminGemmaUsageStats {
  completed_summaries: number
  usage_available: boolean
  prompt_tokens: number
  completion_tokens: number
  thinking_tokens: number
  total_tokens: number
}
export interface SummaryModelMetadata {
  model: string
  runtime: string
  revision: string
  quantization: string
  usage_available: boolean
}
export interface AdminOpsStats {
  generated_at: string
  media: AdminOpsEntityStats
  scan: AdminOpsEntityStats
  transcriptions: AdminOpsTranscriptionStats
  summaries: AdminOpsSummaryStats
  recordings: AdminOpsEntityStats
  providers: { speechmatics: AdminSTTUsageStats; gemma: AdminGemmaUsageStats }
  summary_model?: SummaryModelMetadata
}

export const adminOpsKeys = {
  all: ['admin', 'ops'] as const,
  stats: () => [...adminOpsKeys.all, 'stats'] as const,
}
const emptyEntity = (): AdminOpsEntityStats => ({ total: 0, by_status: {} })
const emptySpeechmatics = (): AdminSTTUsageStats => ({
  submitted_jobs: 0,
  completed_jobs: 0,
  failed_jobs: 0,
  stale_submitted_over_2h: 0,
  processed_audio_seconds: 0,
})
const emptyGemma = (): AdminGemmaUsageStats => ({
  completed_summaries: 0,
  usage_available: false,
  prompt_tokens: 0,
  completion_tokens: 0,
  thinking_tokens: 0,
  total_tokens: 0,
})

function normalize(input: Partial<AdminOpsStats>): AdminOpsStats {
  return {
    generated_at: input.generated_at ?? '',
    media: input.media ?? emptyEntity(),
    scan: input.scan ?? emptyEntity(),
    recordings: input.recordings ?? emptyEntity(),
    transcriptions: {
      ...emptyEntity(),
      ...input.transcriptions,
      stale_submitted_over_2h: input.transcriptions?.stale_submitted_over_2h ?? 0,
    },
    summaries: {
      ...emptyEntity(),
      ...input.summaries,
      stale_pending_over_30m: input.summaries?.stale_pending_over_30m ?? 0,
    },
    providers: {
      speechmatics: { ...emptySpeechmatics(), ...input.providers?.speechmatics },
      gemma: { ...emptyGemma(), ...input.providers?.gemma },
    },
    summary_model: input.summary_model,
  }
}

export function useAdminOpsStats() {
  return useQuery({
    queryKey: adminOpsKeys.stats(),
    queryFn: async () => normalize(await apiClient.get<Partial<AdminOpsStats>>('/admin/ops/stats')),
    staleTime: 30_000,
  })
}
