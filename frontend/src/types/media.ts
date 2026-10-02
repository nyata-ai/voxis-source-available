export interface AudioFinding {
  check: string
  severity: 'high' | 'medium' | 'low' | 'info'
  status: 'pass' | 'fail' | 'warn' | 'skip'
  summary: string
}

export interface AudioAnalysis {
  recording_date?: string
  recording_source?: string
  encoder?: string
  trust_level: 'high' | 'medium' | 'low' | 'unknown'
  findings: AudioFinding[]
  format_name?: string
  codec_name?: string
  sample_rate?: number
  channels?: number
  bitrate?: number
  disclaimer: string
}

export interface MediaItem {
  id: string
  title?: string | null
  description?: string | null
  filename: string
  content_type: string
  size: number
  duration: number
  status: string
  scan_status?: string
  file_hash?: string
  audio_analysis?: AudioAnalysis | null
  encryption_algo?: string
  latest_transcription_id?: string | null
  audio_deleted_at?: string
  audio_available?: boolean
  created_at: string
}

export interface MediaListResponse {
  items: MediaItem[]
  total: number
}

export interface MediaSearchParams {
  search?: string
  status?: string
  limit?: number
  offset?: number
  enabled?: boolean
}

export interface MediaStreamUrlResponse {
  url: string
  expires_at: string
}
