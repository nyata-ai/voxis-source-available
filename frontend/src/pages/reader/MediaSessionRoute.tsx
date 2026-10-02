import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { SessionReaderPage } from '@/components/session/SessionReaderPage'
import { isAudioServable, mediaAudioState } from '@/components/session/audio-state'
import { toLenses } from '@/components/session/lenses'
import { clearAutoBriefingGuard, useAutoBriefing } from '@/components/session/useAutoBriefing'
import { useStreamRetry, type StreamRetry } from '@/components/session/useStreamRetry'
import { isRateLimited, summaryErrorMessage } from '@/components/session/summary-errors'
import type {
  SessionAudio,
  SessionBriefingControls,
  SessionCapabilities,
  SessionView,
} from '@/components/session/session-view'
import { useDeleteMedia, useMediaDetail, useMediaStreamUrl, useUpdateMedia } from '@/hooks/useMedia'
import { usePreferences } from '@/hooks/useSettings'
import {
  summaryKeys,
  useCreateSummary,
  useGenerateAllSummariesForTranscription,
  useRegenerateSummary,
  useSummariesByTranscription,
} from '@/hooks/useSummary'
import { useTranscriptionDetail } from '@/hooks/useTranscription'
import { toast } from '@/lib/toast'
import type { SummaryType } from '@/types/summary'
import type { SummaryProfile } from '@/types/settings'
import type { MediaItem } from '@/types/media'
import type { TranscriptionDetail } from '@/types/transcription'

/**
 * What a media session permits, read off the two rows behind it rather than
 * assumed. Delete removes the whole session, so it follows the media row —
 * DELETE /media/:id cascades through the transcription and its summaries.
 * Export follows the transcription (the endpoints accept any row that loaded),
 * while download and re-transcribe follow the stored file: both hand out or
 * re-read bytes, so a deleted, withheld or infected file must offer neither.
 */
function mediaCapabilities(
  tx: TranscriptionDetail | undefined,
  media: MediaItem | undefined
): SessionCapabilities {
  const hasSession = tx !== undefined
  const audioServable = isAudioServable(media)
  return {
    // The media row owns title and description, and PATCH /media/:id cares about
    // neither transcription status nor scan state — only that the row exists.
    editTitle: Boolean(tx?.media_id),
    editDescription: Boolean(tx?.media_id),
    delete: Boolean(tx?.media_id),
    // The export endpoints are defined for any transcription row, whatever its
    // status, so the only thing to derive is that a row exists.
    export: hasSession,
    downloadAudio: audioServable,
    retranscribe: audioServable,
    // Speaker labels and briefings both key off the transcription id.
    speakers: hasSession,
    briefings: hasSession,
  }
}

/** Everything the reader reads straight off the two rows. Kept apart from the
 *  hook so the adapter below stays about wiring, not field mapping. */
function mediaSessionContent(
  id: string,
  tx: TranscriptionDetail | undefined,
  media: MediaItem | undefined
): Omit<
  SessionView,
  'audio' | 'lenses' | 'briefing' | 'can' | 'actions' | 'isLoading' | 'isError' | 'refetch'
> {
  return {
    source: 'media',
    id,
    transcriptionId: tx?.id,
    mediaId: tx?.media_id,
    status: tx?.status ?? 'pending',
    errorMessage: tx?.error_message,
    // The media row is the newer of the two after an inline edit (the update
    // writes it straight into the cache), so it leads and the transcription's
    // denormalized copy fills in until it loads.
    title: (media?.title ?? tx?.media_title ?? '').trim(),
    titlePlaceholder: tx?.media_filename || (media?.filename ?? ''),
    description: (media?.description ?? tx?.media_description ?? '').trim(),
    preprocessorUsed: tx?.preprocessor_used,
    languages: tx?.languages ?? [],
    durationSeconds: tx?.duration_seconds ?? 0,
    wordCount: tx?.word_count ?? 0,
    speakerCount: tx?.speaker_count ?? 0,
    sizeBytes: media?.size,
    encryptionAlgo: media?.encryption_algo,
    createdAt: tx?.created_at ?? '',
    completedAt: tx?.completed_at,
    utterances: Array.isArray(tx?.utterances) ? tx.utterances : [],
    fullTranscript: tx?.full_transcript,
    speakerMap: tx?.speaker_map,
    suggestedSpeakerMap: tx?.suggested_speaker_map,
    suggestionsGenerated: tx?.suggestions_generated,
  }
}

/**
 * The "Generate all" half of a media session's briefing controls. One request:
 * POST /transcriptions/:id/summaries stopped being url-only, so this path no
 * longer fans out over POST /summaries client-side and no longer has to reason
 * about a partly-rejected batch.
 */
function useGenerateAllBriefings(id: string) {
  const { t } = useTranslation(['summary'])
  const queryClient = useQueryClient()
  const mutation = useGenerateAllSummariesForTranscription()
  const generateAllSummaries = mutation.mutate

  const generateAll = useCallback(
    (summaryProfile?: SummaryProfile) => {
      if (!id) return
      generateAllSummaries(
        { transcriptionId: id, summaryProfile },
        {
          onError: (error) => {
            toast.error(summaryErrorMessage(error, t, 'summary:briefing.error'))
            // A rate-limited attempt failed for want of tokens, not for want of
            // a working endpoint — retract the auto-briefing guard so the next
            // visit may try again rather than leaving four empty sections.
            if (isRateLimited(error)) clearAutoBriefingGuard(id)
            // A partial 500 can persist some rows before the enqueue fails;
            // refetch the list so partially created briefings surface instead
            // of staying hidden. Same reasoning as the URL reader.
            queryClient.invalidateQueries({ queryKey: summaryKeys.byTranscription(id) })
          },
        }
      )
    },
    [generateAllSummaries, id, queryClient, t]
  )

  return { generateAll, isGeneratingAll: mutation.isPending, error: mutation.error }
}

/**
 * The per-type half: one briefing, generated because the reader asked for that
 * one. A single mutation slot, mirroring regenerate — the pane disables every
 * Generate button while one is in flight, so a second type cannot start before
 * the first settles and `variables` cannot describe the wrong request.
 */
function usePerTypeBriefing(id: string) {
  const { t } = useTranslation(['summary'])
  const mutation = useCreateSummary()
  const createSummary = mutation.mutate

  const generateType = useCallback(
    (summaryType: SummaryType, summaryProfile?: SummaryProfile) => {
      if (!id) return
      createSummary(
        { transcription_id: id, summary_type: summaryType, summary_profile: summaryProfile },
        {
          onError: (error) =>
            toast.error(summaryErrorMessage(error, t, 'summary:briefing.generateTypeError')),
        }
      )
    },
    [createSummary, id, t]
  )

  return {
    generateType,
    generatingType: mutation.isPending ? mutation.variables?.summary_type : undefined,
    generateTypeError: mutation.error,
    generateTypeErrorType: mutation.isError ? mutation.variables?.summary_type : undefined,
  }
}

/** The shape the composer reads off the stream query — structural so the helper
 *  below does not have to name a React Query result type. */
interface StreamQuery {
  data?: { url: string }
  isLoading: boolean
  error: Error | null
}

/** The reader's audio rail. `canStream` mirrors the stream query's own enabled
 *  condition: without it a session whose stream was never requested (still
 *  processing, say) could dock a player on a cached URL. */
function mediaSessionAudio(
  media: MediaItem | undefined,
  stream: StreamQuery,
  canStream: boolean,
  retry: StreamRetry
): SessionAudio {
  return {
    streamUrl: canStream ? (stream.data?.url ?? null) : null,
    isLoading: canStream && stream.isLoading,
    state: mediaAudioState(media, stream.error, retry.streamFailed),
    onError: retry.onStreamError,
    onLoaded: retry.onStreamLoaded,
    retry: retry.retryStream,
  }
}

/** The regenerate half of the briefing controls, read off the mutation.
 *  `regeneratingId` is purely in-flight; `regenerateErrorId` outlives the settle
 *  so the pane can keep the inline error on the lens that failed. */
interface RegenerateState {
  isPending: boolean
  isError: boolean
  variables: { id: string } | undefined
  error: unknown
}

function mediaSessionBriefing(
  generation: Omit<
    SessionBriefingControls,
    'regenerate' | 'regeneratingId' | 'regenerateError' | 'regenerateErrorId'
  >,
  regenerate: (summaryId: string, summaryProfile?: SummaryProfile) => void,
  mutation: RegenerateState
): SessionBriefingControls {
  return {
    ...generation,
    regenerate,
    regeneratingId: mutation.isPending ? mutation.variables?.id : undefined,
    regenerateError: mutation.error,
    regenerateErrorId: mutation.isError ? mutation.variables?.id : undefined,
  }
}

/**
 * The Session Reader's adapter for media-backed transcriptions: the
 * transcription row, the media row behind it, its stream URL and its summary
 * list, projected onto `SessionView`. The summary list is content-less, so the
 * lenses carry ids only and the briefing pane fetches each one it opens.
 *
 * Module-private: the route below is the only caller, and exporting a non-
 * component from a component module costs a react-refresh warning for nothing.
 */
function useMediaSessionView(id: string): SessionView {
  const { t } = useTranslation(['summary', 'transcription', 'media'])
  const navigate = useNavigate()
  const { data: tx, isLoading, isError, refetch } = useTranscriptionDetail(id)
  const mediaId = tx?.media_id ?? ''
  const { data: media } = useMediaDetail(mediaId)

  const isCompleted = tx?.status === 'completed'
  // Asking for a stream URL that the server will refuse (deleted, unscanned or
  // infected audio) only produces a 403/409 to swallow, so the query waits.
  const canStream = isCompleted && isAudioServable(media)
  const stream = useMediaStreamUrl(canStream ? mediaId : '')
  const streamRetry = useStreamRetry(id, stream.refetch)

  const summariesQuery = useSummariesByTranscription(isCompleted ? id : '')
  const lenses = toLenses(summariesQuery.data ?? [])
  const { generateAll, isGeneratingAll, error: generateAllError } = useGenerateAllBriefings(id)
  const perType = usePerTypeBriefing(id)
  const regenerateMutation = useRegenerateSummary()
  const updateMedia = useUpdateMedia()
  const deleteMutation = useDeleteMedia()

  const { data: preferences } = usePreferences()
  const can = mediaCapabilities(tx, media)

  // Stable identity: the auto-briefing effect depends on this, and a fresh
  // closure every render would re-run it on every render.
  const autoGenerate = useCallback(() => generateAll(), [generateAll])

  // `isSuccess`, not `!isLoading`: the list request failing looks exactly like a
  // session with no briefings, and only one of those is worth generating for.
  // The preference rides in `enabled`: automatic generation is opt-in.
  const { autoStarted } = useAutoBriefing({
    sessionId: id,
    status: tx?.status ?? 'pending',
    lenses,
    summariesReady: summariesQuery.isSuccess,
    generate: autoGenerate,
    enabled: can.briefings && preferences?.auto_briefings === true,
  })

  const saveMetadata = (data: { title?: string; description?: string }, failedKey: string) => {
    if (!mediaId) return
    updateMedia.mutate({ id: mediaId, data }, { onError: () => toast.error(t(failedKey)) })
  }

  const regenerate = (summaryId: string, summaryProfile?: SummaryProfile) =>
    regenerateMutation.mutate(
      { id: summaryId, summaryProfile },
      {
        onError: (error) =>
          toast.error(summaryErrorMessage(error, t, 'summary:briefing.regenerateError')),
      }
    )

  return {
    ...mediaSessionContent(id, tx, media),
    audio: mediaSessionAudio(media, stream, canStream, streamRetry),
    lenses,
    lensesReady: summariesQuery.isSuccess,
    briefing: mediaSessionBriefing(
      { ...perType, generateAll, isGeneratingAll, error: generateAllError, autoStarted },
      regenerate,
      regenerateMutation
    ),
    can,
    actions: {
      saveTitle: (value) =>
        saveMetadata({ title: value }, 'transcription:detail.updateTitleFailed'),
      saveDescription: (value) =>
        saveMetadata({ description: value }, 'transcription:detail.updateDescriptionFailed'),
      // A silent failure here is the worst outcome: the dialog dismisses itself
      // on confirm either way, so with no error branch the session simply stays
      // in the Library and nothing ever says why.
      delete: () => {
        if (!mediaId) return
        deleteMutation.mutate(mediaId, {
          onSuccess: () => {
            toast.success(t('media:delete.success'))
            navigate('/library')
          },
          onError: () => toast.error(t('media:delete.failed')),
        })
      },
      isDeleting: deleteMutation.isPending,
    },
    forensics: media?.audio_analysis
      ? { analysis: media.audio_analysis, fileHash: media.file_hash }
      : undefined,
    retranscribeSource: can.retranscribe ? tx : undefined,
    latestTranscriptionId: media?.latest_transcription_id,
    isLoading,
    // A settled query with no row is a 404 in every way that matters here.
    isError: isError || (!isLoading && !tx),
    refetch: () => {
      void refetch()
    },
  }
}

export function MediaSessionRoute() {
  const { id } = useParams<{ id: string }>()
  const session = useMediaSessionView(id ?? '')

  return <SessionReaderPage session={session} />
}
