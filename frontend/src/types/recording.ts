/** Recording session status as reported by the backend. */
export type RecordingSessionStatus =
  | 'recording'
  | 'paused'
  | 'completing'
  | 'completed'
  | 'interrupted'
  | 'abandoned'
  | 'failed'

/** Frontend-only UI status for the recording store. */
export type RecordingUIStatus = 'idle' | 'recording' | 'paused' | 'completing' | 'error'

/** MIME type negotiated for recording. */
export type RecordingMimeType = 'audio/webm' | 'audio/mp4'

/** Browser capture source for a live recording. */
export type RecordingCaptureSource = 'microphone' | 'mixed_audio'

/** Recording session entity from the backend. */
export interface RecordingSession {
  id: string
  organization_id: string
  user_id: string
  status: RecordingSessionStatus
  mime_type: string
  microphone_label?: string
  media_id?: string
  total_duration: number
  capture_source?: RecordingCaptureSource
  /** Number of chunks the server has stored for the session. */
  chunk_count?: number
  /** Timestamp of the newest stored chunk (null when no chunk arrived yet). */
  last_chunk_at?: string | null
  last_activity_at?: string | null
  created_at: string
  updated_at: string
  completed_at?: string
}

/** Request payload to create a recording session. */
export interface CreateRecordingRequest {
  mime_type: string
  microphone_label?: string
  capture_source?: RecordingCaptureSource
}

/** Request payload to upload a chunk. */
export interface UploadChunkRequest {
  sessionId: string
  seq: number
  blob: Blob
}

/** Response from GET /recordings/interrupted. */
export interface InterruptedRecordingsResponse {
  items: RecordingSession[]
}

/** IndexedDB outbox entry for a chunk. */
export interface OutboxEntry {
  id?: number
  sessionId: string
  seq: number
  blob: Blob
  createdAt: number
}

/** Encrypted IndexedDB representation for a recording chunk. */
export interface StoredOutboxEntry {
  id?: number
  sessionId: string
  seq: number
  encryptedBlob: Blob
  iv: number[]
  mimeType: string
  createdAt: number
}

/** Non-extractable Web Crypto key persisted for a recording outbox session. */
export interface StoredOutboxKey {
  sessionId: string
  key: CryptoKey
}

/** Which signed-in user (Keycloak `sub`) the buffered recording outbox belongs to. */
export interface StoredOutboxOwner {
  name: 'owner'
  userId: string
}

/** Codec negotiation result. */
export interface CodecResult {
  supported: boolean
  mimeType: RecordingMimeType | null
  codecString: string | null
}
