export type ActivityKind = 'recording' | 'transcription'
export type ActivityStage = 'stitching' | 'transcribing' | 'summarizing' | 'ready'
export type ActivityStatus = 'in_progress' | 'completed' | 'failed'

export interface ActivityDetail {
  terminal: number
  total: number
}

export interface ActivityItem {
  kind: ActivityKind
  ref_id: string
  link?: string
  title?: string
  stage: ActivityStage
  status: ActivityStatus
  error_message?: string
  detail?: ActivityDetail
  started_at: string
}

export interface ActivityResponse {
  items: ActivityItem[]
}
