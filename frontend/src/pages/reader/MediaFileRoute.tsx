import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { FileText } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { SessionReaderPage } from '@/components/session/SessionReaderPage'
import { isAudioServable, mediaAudioState } from '@/components/session/audio-state'
import { useStreamRetry } from '@/components/session/useStreamRetry'
import type {
  SessionBriefingControls,
  SessionCapabilities,
  SessionView,
} from '@/components/session/session-view'
import { TranscribeButton } from '@/components/transcription/TranscribeButton'
import { useDeleteMedia, useMediaDetail, useMediaStreamUrl, useUpdateMedia } from '@/hooks/useMedia'
import { toast } from '@/lib/toast'
import type { MediaItem } from '@/types/media'

/** Nothing to brief: an untranscribed file has no transcription id to hang a
 *  summary on. The controls exist only because the contract requires them, and
 *  `can.briefings: false` keeps the pane that would use them off the page. */
const NO_BRIEFING: SessionBriefingControls = {
  generateType: () => {},
  generateTypeError: null,
  generateAll: () => {},
  isGeneratingAll: false,
  error: null,
  regenerate: () => {},
  regenerateError: null,
}

/**
 * What an uploaded file that has not been transcribed permits, read off the
 * media row. Title, description and delete need only that the row exists —
 * PATCH and DELETE /media/:id care about neither status nor scan state — while
 * download follows the stored file, so a deleted, withheld or infected one
 * offers nothing. The four that are structurally false stay false: there is no
 * transcript to export, no speakers to label, no briefing to generate, and a
 * first transcription is not a re-transcription.
 */
function mediaFileCapabilities(media: MediaItem | undefined): SessionCapabilities {
  const exists = media !== undefined
  return {
    editTitle: exists,
    editDescription: exists,
    delete: exists,
    export: false,
    downloadAudio: isAudioServable(media),
    retranscribe: false,
    speakers: false,
    briefings: false,
  }
}

/**
 * Media lifecycle → the reader's status vocabulary. The shell and the buttons
 * it shares with the other two adapters speak transcription statuses, so a file
 * still being sealed reads as `processing` (in flight, nothing to show) and a
 * settled one as `completed`. Anything else — `pending`, `failed`, a status
 * added later — already means the same thing on both sides and passes through.
 */
function readerStatus(mediaStatus: string | undefined): string {
  if (mediaStatus === 'ready') return 'completed'
  if (mediaStatus === 'encrypting') return 'processing'
  return mediaStatus ?? 'pending'
}

/** Everything the reader reads straight off the media row. Kept apart from the
 *  hook so the adapter below stays about wiring, not field mapping. */
function mediaFileContent(
  id: string,
  media: MediaItem | undefined
): Omit<
  SessionView,
  'audio' | 'lenses' | 'briefing' | 'can' | 'actions' | 'isLoading' | 'isError' | 'refetch'
> {
  return {
    source: 'untranscribed',
    id,
    mediaId: id,
    status: readerStatus(media?.status),
    title: (media?.title ?? '').trim(),
    titlePlaceholder: media?.filename ?? '',
    description: (media?.description ?? '').trim(),
    languages: [],
    durationSeconds: media?.duration ?? 0,
    // Nothing has been transcribed, so there are no words, no speakers and no
    // segments to count or render.
    wordCount: 0,
    speakerCount: 0,
    utterances: [],
    sizeBytes: media?.size,
    encryptionAlgo: media?.encryption_algo,
    createdAt: media?.created_at ?? '',
    latestTranscriptionId: media?.latest_transcription_id,
  }
}

/** Resolves an i18n key — namespaced or bare — to its localized string. */
type TranslateFn = (key: string) => string

/**
 * The transcript pane's stand-in for a file that has no transcript: open the
 * session this file already produced, or start its first. Both answers come off
 * the media row.
 *
 * A pending malware scan is the one blocked state that is only a wait, so it
 * gets its own answer rather than an empty column: the sentence says the scan
 * is running and the disabled button says what will be possible when it lands.
 * The detail query keeps polling through `scan_pending`, so the real button
 * arrives on its own. The terminal refusals (infected, scan error, deleted
 * audio) still render nothing here — the audio rail already names them, and
 * there is no action to offer.
 */
function transcriptCta(id: string, media: MediaItem | undefined, t: TranslateFn): ReactNode {
  if (!media || media.status !== 'ready') return undefined

  if (media.latest_transcription_id) {
    return (
      <Button asChild>
        <Link to={`/transcriptions/${media.latest_transcription_id}`}>
          <FileText className="mr-2 h-4 w-4" />
          {t('detail.viewTranscription')}
        </Link>
      </Button>
    )
  }

  if (media.scan_status === 'scan_pending') {
    return (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">{t('errors:codes.scan_pending')}</p>
        <Button disabled>
          <FileText className="mr-2 h-4 w-4" />
          {t('transcription:transcribe.button')}
        </Button>
      </div>
    )
  }

  if (!isAudioServable(media)) return undefined
  return <TranscribeButton mediaId={id} mediaStatus={media.status} />
}

interface MediaFileView {
  session: SessionView
  media: MediaItem | undefined
}

/**
 * The Session Reader's adapter for an uploaded file with no transcription: one
 * media query plus its stream URL and the two mutations the row supports,
 * projected onto `SessionView`. Everything transcript-shaped is empty by
 * construction; the transcript pane carries a call to action instead.
 *
 * Module-private: the route below is the only caller, and exporting a non-
 * component from a component module costs a react-refresh warning for nothing.
 */
function useMediaFileView(id: string): MediaFileView {
  // `media` stays the default namespace so the bare keys below still resolve;
  // the CTA reaches into `transcription` and `errors` with explicit prefixes.
  const { t } = useTranslation(['media', 'transcription', 'errors'])
  const navigate = useNavigate()
  const { data: media, isLoading, isError, refetch } = useMediaDetail(id)

  // Asking for a stream URL the server will refuse (still encrypting, deleted,
  // unscanned or infected audio) only produces an error to swallow.
  const canStream = media?.status === 'ready' && isAudioServable(media)
  const stream = useMediaStreamUrl(canStream ? id : '')
  const streamRetry = useStreamRetry(id, stream.refetch)

  const updateMedia = useUpdateMedia()
  const deleteMedia = useDeleteMedia()

  const saveMetadata = (data: { title?: string; description?: string }, failedKey: string) => {
    if (!id) return
    updateMedia.mutate({ id, data }, { onError: () => toast.error(t(failedKey)) })
  }

  return {
    media,
    session: {
      ...mediaFileContent(id, media),
      audio: {
        streamUrl: canStream ? (stream.data?.url ?? null) : null,
        isLoading: canStream && stream.isLoading,
        state: mediaAudioState(media, stream.error, streamRetry.streamFailed),
        onError: streamRetry.onStreamError,
        onLoaded: streamRetry.onStreamLoaded,
        retry: streamRetry.retryStream,
      },
      lenses: [],
      // An untranscribed row has no briefings to wait for.
      lensesReady: true,
      briefing: NO_BRIEFING,
      can: mediaFileCapabilities(media),
      actions: {
        saveTitle: (value) => saveMetadata({ title: value }, 'detail.updateTitleError'),
        saveDescription: (value) =>
          saveMetadata({ description: value }, 'detail.updateDescriptionError'),
        // The dialog dismisses itself on confirm either way, so without an error
        // branch a refused delete leaves the file in place and says nothing.
        delete: () => {
          if (!id) return
          deleteMedia.mutate(id, {
            onSuccess: () => navigate('/library'),
            onError: () => toast.error(t('delete.failed')),
          })
        },
        isDeleting: deleteMedia.isPending,
      },
      forensics: media?.audio_analysis
        ? { analysis: media.audio_analysis, fileHash: media.file_hash }
        : undefined,
      transcriptCta: transcriptCta(id, media, t),
      isLoading,
      // A settled query with no row is a 404 in every way that matters here.
      isError: isError || (!isLoading && !media),
      refetch: () => {
        void refetch()
      },
    },
  }
}

export function MediaFileRoute() {
  const { id } = useParams<{ id: string }>()
  const { session } = useMediaFileView(id ?? '')

  return <SessionReaderPage session={session} />
}
