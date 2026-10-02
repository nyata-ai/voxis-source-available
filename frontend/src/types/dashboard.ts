export interface DashboardStats {
  total_media: number
  processing_count: number
  encrypting_count: number
  transcribing_count: number
  completed_transcriptions: number
  completed_this_month: number
  completed_last_month: number
  seconds_this_month: number
  last_completed_at?: string | null
}
