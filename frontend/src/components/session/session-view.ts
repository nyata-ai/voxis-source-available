import type { ReactNode } from 'react'
import type { SummaryDetail, SummaryType } from '@/types/summary'
import type { SummaryProfile } from '@/types/settings'
import type { SpeakerSuggestion, TranscriptionItem, Utterance } from '@/types/transcription'
import type { AudioAnalysis } from '@/types/media'

/**
 * The Session Reader's contract. Two adapters produce a `SessionView` — a
 * media-backed transcription and an untranscribed media row — and one set of
 * components renders it. Types only: this module
 * must stay free of runtime code so adapters and components can import it
 * without pulling anything into the bundle.
 *
 * A discriminated union over `source` was considered and rejected: the three
 * sources differ in a dozen small ways, not in shape, and a union would push
 * every consumer through narrowing it does not need. **The rule that replaces
 * it: every `can.*` flag MUST be derived from data — `audio_available`,
 * `audio_deleted_at`, scan state, source type, status — never a constant
 * `true`.** A hardcoded capability is how a reader offers a download for audio
 * that no longer exists. If an adapter cannot answer a capability from data, it
 * answers `false`.
 */
export type SessionSource = 'media' | 'untranscribed'

/** One briefing lens. The pane fetches by `summaryId` because
 *  GET /transcriptions/:id/summaries is content-less. */
export interface SessionLens {
  type: SummaryType
  summaryId?: string
  status?: string
  summary?: SummaryDetail
}

/** Playback state for the reader's audio rail. `streamUrl` is null whenever the
 *  audio is not playable — `state` says why. */
export interface SessionAudio {
  streamUrl: string | null
  isLoading: boolean
  state: 'available' | 'unavailable' | 'deleted' | 'scan_pending' | 'scan_blocked' | 'failed'
  onError: () => void
  /** The player reporting that the current URL loaded. Retiring the retry
   *  budget is the point: without it the count only ever spends down and a long
   *  session eventually reports failed audio that recovered every time. */
  onLoaded?: () => void
  /** Refetch the signed URL and restore the retry budget for the rail's manual
   *  retry once `state` is `failed`. */
  retry?: () => void
}

/**
 * Briefing generation controls. Three verbs, deliberately distinct:
 * `generateType` produces one briefing the user asked for, `generateAll` fills
 * every generatable lens in one round-trip, and `regenerate` replaces a single
 * existing summary. Each carries its own in-flight and error state so the pane
 * can put a failure where it belongs — a per-type failure under its section, a
 * generate-all failure above all four.
 *
 * Every verb takes the profile the pane currently has selected. Adapters omit
 * the argument when professional profiles are disabled for the deployment, so
 * the server resolves the user's saved preference instead.
 */
export interface SessionBriefingControls {
  /** Generates one briefing type for this session. */
  generateType: (summaryType: SummaryType, summaryProfile?: SummaryProfile) => void
  /** The type whose generate request is in flight — purely in-flight state, one
   *  slot, cleared on settle either way. */
  generatingType?: SummaryType
  /** The error from the most recent per-type generate. Scoped by
   *  `generateTypeErrorType`, which outlives the settle so the section that
   *  failed keeps its inline message. */
  generateTypeError: unknown
  generateTypeErrorType?: SummaryType
  /** Fills every generatable lens at once. */
  generateAll: (summaryProfile?: SummaryProfile) => void
  isGeneratingAll: boolean
  /** The error from the most recent generate-all. Pane-level: the run covered
   *  every section, so no single one owns the failure. */
  error: unknown
  regenerate: (summaryId: string, summaryProfile?: SummaryProfile) => void
  /** The summaryId of the lens currently regenerating — purely in-flight state.
   *  Undefined once the attempt settles, success or failure. */
  regeneratingId?: string
  /** True when this visit started generation on its own rather than on a click,
   *  so the pane can account for work the user did not ask for. */
  autoStarted?: boolean
  regenerateError: unknown
  /** The summaryId of the lens whose most recent regenerate attempt failed.
   *  Undefined when none has failed, or once a new regenerate has started for
   *  any lens — starting a new attempt clears the previous error. */
  regenerateErrorId?: string
}

/** Audio provenance, present only for media-backed sessions that were analyzed. */
export interface SessionForensics {
  analysis: AudioAnalysis
  fileHash?: string
}

/** What this session permits. Every flag is data-derived — see the note on
 *  `SessionSource`. Never hardcode one to `true`. */
export interface SessionCapabilities {
  editTitle: boolean
  editDescription: boolean
  delete: boolean
  export: boolean
  downloadAudio: boolean
  retranscribe: boolean
  speakers: boolean
  briefings: boolean
}

export interface SessionActions {
  saveTitle: (v: string) => void
  saveDescription: (v: string) => void
  delete?: () => void
  isDeleting?: boolean
}

export interface SessionView {
  source: SessionSource
  /** Route param: a transcription id or a media id, depending on `source`. */
  id: string
  transcriptionId?: string
  mediaId?: string
  status: string
  errorMessage?: string
  title: string
  titlePlaceholder: string
  description: string
  preprocessorUsed?: string
  languages: string[]
  durationSeconds: number
  wordCount: number
  speakerCount: number
  sizeBytes?: number
  /** The stored file's encryption cipher (e.g. `AES-256-GCM`), read straight off
   *  the media row. Drives the reader's Encrypted indicator. */
  encryptionAlgo?: string
  createdAt: string
  completedAt?: string
  utterances: Utterance[]
  /** What the transcript pane shows when a session has no transcript to show:
   *  the untranscribed adapter's call to action (start the first transcription,
   *  or open the one this file already produced). Rendered *instead of* the
   *  viewer, never beside it, so an adapter with real utterances leaves it
   *  undefined. A node rather than a flag because the actions belong to the
   *  adapter's source — the shell must not learn what a media file can start. */
  transcriptCta?: ReactNode
  fullTranscript?: string
  speakerMap?: Record<string, string>
  suggestedSpeakerMap?: Record<string, SpeakerSuggestion>
  suggestionsGenerated?: boolean
  audio: SessionAudio
  lenses: SessionLens[]
  /** Whether `lenses` reflects a settled answer from the server rather than the
   *  placeholder set every adapter starts from. The reader waits for it before
   *  deciding which briefing to open — resolving against a list that has not
   *  arrived picks a tab and then has to move it. Optional: absent reads as
   *  settled, which is right for an adapter with no briefings to fetch. */
  lensesReady?: boolean
  briefing: SessionBriefingControls
  forensics?: SessionForensics
  can: SessionCapabilities
  actions: SessionActions
  /** The row a re-transcribe dialog needs; absent when `can.retranscribe` is false. */
  retranscribeSource?: TranscriptionItem
  latestTranscriptionId?: string | null
  isLoading: boolean
  isError: boolean
  refetch: () => void
}
