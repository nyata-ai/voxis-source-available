import type { SummaryProfile } from '@/types/settings'

export type SummaryType = 'general' | 'key_points' | 'action_items' | 'q_and_a'
export type SummaryReviewStatus =
  | 'skipped'
  | 'pending'
  | 'retrying'
  | 'passed'
  | 'flagged'
  | 'failed'
  | 'parse_failed'
  | 'safety_blocked'

export type SummaryActionKind = 'action_item' | 'decision' | 'next_step' | 'open_issue'
export type SummaryAnswerStatus = 'answered' | 'partially_answered' | 'conflicting' | 'unanswered'

export interface StructuredSummaryParagraph {
  id: string
  text: string
  speaker: string | null
  citation_ids: string[]
}

export type StructuredSummaryKeyPoint = StructuredSummaryParagraph

export interface StructuredSummaryActionItem extends StructuredSummaryParagraph {
  kind: SummaryActionKind
  owner: string | null
  deadline: string | null
}

export interface StructuredSummaryQuestionAnswer {
  id: string
  question: string
  question_speaker: string | null
  answer: string
  answer_speaker: string | null
  answer_status: SummaryAnswerStatus
  answer_exact_quote: string | null
  citation_ids: string[]
}

interface StructuredSummaryBase {
  schema_version: string
  matrix_language: string
}

export interface StructuredGeneralSummary extends StructuredSummaryBase {
  paragraphs: StructuredSummaryParagraph[]
}

export interface StructuredKeyPointsSummary extends StructuredSummaryBase {
  items: StructuredSummaryKeyPoint[]
}

export interface StructuredActionItemsSummary extends StructuredSummaryBase {
  items: StructuredSummaryActionItem[]
}

export interface StructuredQuestionAndAnswerSummary extends StructuredSummaryBase {
  items: StructuredSummaryQuestionAnswer[]
}

export type StructuredSummaryContent =
  | StructuredGeneralSummary
  | StructuredKeyPointsSummary
  | StructuredActionItemsSummary
  | StructuredQuestionAndAnswerSummary

/** Additive source details for citation chips. Legacy responses omit this. */
export interface SummaryCitation {
  id: string
  speaker?: string | null
  start_seconds?: number | null
  end_seconds?: number | null
}

/** Additive generation metadata. Legacy responses omit this. */
export interface SummaryGenerationMetadata {
  prompt_version?: string | null
  model?: string | null
  endpoint_location?: string | null
  source_version?: string | null
  source_hash?: string | null
  structured_schema_version?: string | null
  degradation_codes?: string[] | null
}

export interface SummaryItem {
  id: string
  organization_id: string
  transcription_id: string
  summary_type: SummaryType
  status: string
  word_count: number
  prompt_tokens: number
  completion_tokens: number
  high_stakes: boolean
  /** Absent for summaries created before professional profiles were introduced. */
  summary_profile?: SummaryProfile
  generation_metadata?: SummaryGenerationMetadata | null
  review_status: SummaryReviewStatus
  error_message?: string
  created_at: string
  completed_at?: string
  transcription_media_filename?: string
  transcription_audio_name?: string
  transcription_media_description?: string
}

export interface SummaryDetail extends SummaryItem {
  content?: string
  structured_content?: StructuredSummaryContent | null
  citations?: SummaryCitation[]
}

export interface SummaryListResponse {
  items: SummaryItem[]
  total: number
}

export interface CreateSummaryRequest {
  transcription_id: string
  summary_type: SummaryType
  high_stakes?: boolean
  /** Professional profile for this summary only. Omitted when the deployment
   *  has profiles disabled, so the server falls back to the saved preference. */
  summary_profile?: SummaryProfile
}
