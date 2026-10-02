export type SearchEntityType = 'media'

export interface SearchItem {
  id: string
  entity_type: SearchEntityType
  title: string
  description?: string
  filename?: string
  status: string
  created_at: string
  latest_transcription_id?: string
  score: number
  match_fields?: string[]
  snippet?: string
}

export interface SearchResponse {
  items: SearchItem[]
  total: number
  query: string
  search_mode: 'lexical'
  semantic_available: boolean
  transcript_items: SearchItem[]
  next_transcript_cursor?: string
  transcript_complete: boolean
}

export interface SearchParams {
  search?: string
  limit?: number
  offset?: number
  transcriptCursor?: string
  enabled?: boolean
}
