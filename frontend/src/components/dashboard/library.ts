import type { MediaItem } from '@/types/media'
import type { TranscriptionItem } from '@/types/transcription'

function getTranscriptionTimestamp(item: TranscriptionItem): number {
  return Date.parse(item.completed_at || item.created_at || '') || 0
}

function getMediaTimestamp(item: MediaItem): number {
  return Date.parse(item.created_at || '') || 0
}

export interface LibraryItem {
  key: string
  title: string
  to: string
  timestamp: number
  status: string
  kind: 'media' | 'transcription'
  variant: 'audio'
  /** Always false here; privileged recording is not part of this edition. */
  privilege: boolean
  languages: string[]
  durationSeconds?: number
  retranscribable: boolean
  speakerCount?: number
  wordCount?: number
  dateValue?: string
  /** The transcription's ID when the row has one, else the media ID. */
  signature: string
  mediaId?: string
}

/** A stuck or failed scan matters more than the row's other status. */
const PRIORITY_SCAN_STATUSES = new Set(['scan_pending', 'scan_infected', 'scan_error'])

function mediaRowStatus(m: MediaItem, t?: TranscriptionItem): string {
  if (m.scan_status && PRIORITY_SCAN_STATUSES.has(m.scan_status)) return m.scan_status
  return t?.status || m.status
}

export function buildLibrary(media: MediaItem[], transcriptions: TranscriptionItem[]): LibraryItem[] {
  const consumedMediaIds = new Set<string>()
  const latestPerMedia = new Map<string, TranscriptionItem>()

  for (const t of transcriptions) {
    if (!t.media_id) continue
    const existing = latestPerMedia.get(t.media_id)
    if (!existing || getTranscriptionTimestamp(t) > getTranscriptionTimestamp(existing)) {
      latestPerMedia.set(t.media_id, t)
    }
  }

  const items: LibraryItem[] = []

  for (const m of media) {
    const t = latestPerMedia.get(m.id)
    consumedMediaIds.add(m.id)
    items.push({
      key: `media-${m.id}`,
      title: m.title || m.filename,
      to: t ? `/transcriptions/${t.id}` : `/media/${m.id}`,
      timestamp: Math.max(getMediaTimestamp(m), t ? getTranscriptionTimestamp(t) : 0),
      status: mediaRowStatus(m, t),
      kind: t ? 'transcription' : 'media',
      variant: 'audio',
      privilege: false,
      retranscribable: Boolean(t) && m.status === 'ready',
      languages: t?.languages ?? [],
      durationSeconds: t?.duration_seconds ?? m.duration,
      speakerCount: t?.speaker_count,
      wordCount: t?.word_count,
      dateValue: t?.completed_at || t?.created_at || m.created_at,
      signature: t?.id || m.id,
      mediaId: m.id,
    })
  }

  for (const t of transcriptions) {
    if (t.media_id && consumedMediaIds.has(t.media_id)) continue
    items.push({
      key: `trans-${t.id}`,
      title: t.media_title || t.media_filename || '',
      to: `/transcriptions/${t.id}`,
      timestamp: getTranscriptionTimestamp(t),
      status: t.status,
      kind: 'transcription',
      variant: 'audio',
      privilege: false,
      retranscribable: t.media_status === 'ready',
      languages: t.languages ?? [],
      durationSeconds: t.duration_seconds,
      speakerCount: t.speaker_count,
      wordCount: t.word_count,
      dateValue: t.completed_at || t.created_at,
      signature: t.id,
      mediaId: t.media_id || undefined,
    })
  }

  return items.sort((a, b) => b.timestamp - a.timestamp)
}

/**
 * The session the Desk leads with: the newest completed transcription. The list
 * is the whole organization's, so it can be a colleague's: the card says
 * "Latest session" and never "where you left off".
 */
export function pickContinueItem(items: LibraryItem[]): LibraryItem | undefined {
  return items.find((item) => item.kind === 'transcription' && item.status === 'completed')
}
