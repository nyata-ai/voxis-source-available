export interface TranscriptionItem {
  id: string
  organization_id: string
  media_id: string
  media_filename: string
  media_title: string
  media_description: string
  media_status: string
  status: string
  languages: string[]
  diarization: boolean
  speaker_count: number
  word_count: number
  duration_seconds: number
  enhance_audio?: boolean
  /**
   * Speaker count declared at submission, absent when left on auto-detect.
   * This is the REQUEST value — `speaker_count` is what the provider found.
   */
  expected_speakers?: number
  error_message?: string
  created_at: string
  completed_at?: string
  preprocessor_used?: string
}

/** AI-generated speaker name suggestion with supporting evidence. */
export interface SpeakerSuggestion {
  name: string
  evidence: string
  confidence: 'high' | 'medium' | 'low'
}

export interface TranscriptionDetail extends TranscriptionItem {
  full_transcript?: string
  utterances?: Utterance[]
  speaker_map?: Record<string, string>
  suggested_speaker_map?: Record<string, SpeakerSuggestion>
  suggestions_generated?: boolean
}

export interface Utterance {
  id: number
  speaker: number
  // Speechmatics tags language per word rather than per utterance, so a
  // code-switched utterance may have no single dominant language.
  language?: string
  start: number
  end: number
  text: string
  // Optional: Speechmatics (Melia) returns no confidence scores. Absent means
  // "not provided by this provider" — never render it as 0 or NaN.
  confidence?: number
  words: Word[]
}

export interface Word {
  word: string
  start: number
  end: number
  // Optional: Speechmatics (Melia) returns no confidence scores. Absent means
  // "not provided by this provider" — never render it as 0 or NaN.
  confidence?: number
  // Optional: Melia's per-word language tag (not yet rendered by the UI).
  language?: string
}

export interface TranscriptionListResponse {
  items: TranscriptionItem[]
  total: number
}

export interface CreateTranscriptionRequest {
  media_id: string
  languages: string[]
  diarization: boolean
  min_speakers?: number
  max_speakers?: number
  enhance_audio?: boolean
  /**
   * Declared number of speakers (1..10). Omit to let the provider auto-detect.
   * Only meaningful with diarization on; the server ignores it otherwise.
   */
  expected_speakers?: number
}
