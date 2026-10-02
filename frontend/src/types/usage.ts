export type UserStorageState = 'unlimited' | 'ok' | 'warning' | 'full'

export interface UserStorageQuota {
  used_bytes: number
  limit_bytes: number
  remaining_bytes: number
  usage_percent: number
  warning_threshold_percent: number
  state: UserStorageState
  enforced: boolean
}

export interface UsageStats {
  total_duration_seconds: number
  total_prompt_tokens: number
  total_completion_tokens: number
  total_thinking_tokens: number
  live_recording_retention_enabled: boolean
  live_recording_retention_days: number
  storage_used_bytes: number
  user_storage: UserStorageQuota
}
