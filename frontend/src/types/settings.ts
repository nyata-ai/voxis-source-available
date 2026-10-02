export const SUMMARY_PROFILES = [
  'general_professional',
  'legal',
  'investment_analysis',
  'journalism',
  'negotiation',
  'decision_committee',
  'investigation',
] as const

export type SummaryProfile = (typeof SUMMARY_PROFILES)[number]

export const DEFAULT_SUMMARY_PROFILE: SummaryProfile = 'general_professional'

export function normalizeSummaryProfile(value: unknown): SummaryProfile {
  for (const profile of SUMMARY_PROFILES) {
    if (value === profile) return profile
  }
  return DEFAULT_SUMMARY_PROFILE
}

export interface UserPreferences {
  theme: 'light' | 'dark' | 'system'
  default_languages: string[]
  default_diarization: boolean | null
  default_summary_type: 'general' | 'key_points' | 'action_items' | 'q_and_a'
  default_export_format: 'pdf' | 'docx' | 'json'
  playback_speed: number
  high_stakes_summaries: boolean
  /** Whether opening a completed session generates every briefing type without
   *  being asked. Off by default: generation is billable AI work and not every
   *  recording wants all four briefings. */
  auto_briefings: boolean
  /** Absent for preferences saved before professional profiles were introduced. */
  summary_profile?: SummaryProfile
  /** Absent when the user has never chosen — detection decides in that case. */
  ui_language?: string
}
