import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'

export interface FeatureFlags {
  enhance_audio: boolean
  deep_filter: boolean
  summary_high_stakes: boolean
  summary_structured_output: boolean
  summary_profiles: boolean
  summary_shared_analysis: boolean
  account_security: boolean
  /** Always false in Voxis-OSS; Melia 1 has no Custom dictionary support. */
  custom_vocabulary: boolean
  /** BAP (Berita Acara Pemeriksaan) draft DOCX export template (BAP_EXPORT_ENABLED). */
  bap_export: boolean
}

export const featureKeys = {
  all: ['features'] as const,
}

export function useFeatures() {
  return useQuery({
    queryKey: featureKeys.all,
    queryFn: () => apiClient.get<FeatureFlags>('/features'),
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
  })
}
