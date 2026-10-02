import { useTranslation } from 'react-i18next'
import { Navigate, useParams } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { useSummaryDetail } from '@/hooks/useSummary'
import { useTranscriptionDetail } from '@/hooks/useTranscription'

/**
 * Resolves a standalone briefing link onto the session that owns it.
 *
 * A briefing has no page of its own any more — it is one lens of a session — so
 * every `/summaries/:id` link, bookmarked or shared, has to land on that
 * session's reader with the lens already selected.
 *
 * Two reads, both on the success path. `GET /transcriptions/:id` serves
 * URL-sourced rows as well as media-backed ones (there is no source guard on
 * it), and an empty `media_id` is exactly what distinguishes them — so one
 * request settles which reader owns this summary, with no probing and no 404 to
 * interpret. Anything that fails to load goes to the Library rather than to a
 * dead end: the summary may have been deleted along with its session.
 *
 * Both hops replace. A redirect the user never chose must not become a Back
 * press that lands them right back on it.
 */
export function SummaryRedirect() {
  const { t } = useTranslation(['summary'])
  const { id } = useParams<{ id: string }>()

  const { data: summary, isError } = useSummaryDetail(id ?? '')
  const { data: parent, isError: parentError } = useTranscriptionDetail(
    summary?.transcription_id ?? ''
  )

  if (!id || isError || parentError) return <Navigate to="/library" replace />

  // A summary that loaded without a `transcription_id` has nothing to resolve
  // to: the parent query stays disabled on an empty id, so `parent` never
  // arrives and never errors, and the spinner below would spin forever.
  if (summary && !summary.transcription_id) return <Navigate to="/library" replace />

  if (!summary || !parent) {
    return (
      <div className="flex justify-center py-16" role="status">
        <span className="sr-only">{t('summary:detail.loading')}</span>
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" aria-hidden="true" />
      </div>
    )
  }

  const base = '/transcriptions'
  return <Navigate to={`${base}/${parent.id}?lens=${summary.summary_type}`} replace />
}
