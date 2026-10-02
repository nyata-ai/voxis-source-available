import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, apiClient } from '@/lib/api-client'
import { toast } from '@/lib/toast'

/** The three formats the backend actually renders. There is no TXT export. */
export type ExportFormat = 'pdf' | 'docx' | 'json'

/** The BAP draft is one more choice rather than a fourth format: it always
 *  resolves to `format=docx&template=bap` and carries its own filename hint
 *  and 400/409 error copy. */
export type ExportChoice = ExportFormat | 'bap'

export type ExportEntityType = 'transcription' | 'summary'

export const EXPORT_FORMATS: readonly ExportFormat[] = ['pdf', 'docx', 'json']

export interface ExportRequest {
  entityType: ExportEntityType
  entityId: string
  /** Used only for the client-side fallback filename; the server's
   *  Content-Disposition wins when present. Optional: a caller with no name to
   *  offer gets the entity type instead. */
  mediaFilename?: string
  choice: ExportChoice
}

export interface ResolvedExport {
  endpoint: string
  filename: string
  isBap: boolean
}

/**
 * Turns one export request into the endpoint and fallback filename it needs.
 * Pure, and the single place either caller (the dialog or the panel's pills)
 * derives a URL — a second copy is how `template=bap` ends up on a summary.
 */
export function resolveExport({
  entityType,
  entityId,
  mediaFilename,
  choice,
}: ExportRequest): ResolvedExport {
  // The BAP template only exists on the transcription endpoint, so a `bap`
  // choice against a summary degrades to a plain DOCX rather than sending a
  // parameter that endpoint would reject.
  const isBap = choice === 'bap' && entityType === 'transcription'
  const format: ExportFormat = choice === 'bap' ? 'docx' : choice
  const baseName = mediaFilename?.replace(/\.[^.]+$/, '') || entityType
  // Real ids are UUIDs, so this encodes to itself — but the id reaches here
  // from a route param and an API payload, and neither is this function's to
  // trust with the shape of the path it is spliced into.
  const id = encodeURIComponent(entityId)
  const endpoint =
    entityType === 'transcription'
      ? `/transcriptions/${id}/export?format=${format}${isBap ? '&template=bap' : ''}`
      : `/summaries/${id}/export?format=${format}`
  return {
    endpoint,
    filename: isBap ? `${baseName}-BAP-draf.docx` : `${baseName}.${format}`,
    isBap,
  }
}

export interface ExportRunner {
  /** The caller-supplied key of the download in flight, or null when idle. */
  pendingKey: string | null
  isExporting: boolean
  /** Runs one export. Resolves true only on a completed download, so a dialog
   *  can close itself and a failed pill can stay put. */
  runExport: (request: ExportRequest, key?: string) => Promise<boolean>
}

/**
 * The export request itself — fetch, blob download, toasts — with no opinion
 * about the control that started it. `pendingKey` is what lets a panel of pills
 * disable exactly the one that is downloading; a dialog passes no key and reads
 * `isExporting`.
 */
export function useExportRequest(): ExportRunner {
  const { t } = useTranslation('common')
  const [pendingKey, setPendingKey] = useState<string | null>(null)
  // A ref, not the state, guards re-entry: two clicks in one tick would both
  // read the pre-render state and both start a download.
  const inFlight = useRef(false)

  const runExport = useCallback(
    async (request: ExportRequest, key = 'export'): Promise<boolean> => {
      if (!request.entityId) return false
      if (inFlight.current) return false
      inFlight.current = true
      setPendingKey(key)
      const { endpoint, filename, isBap } = resolveExport(request)
      try {
        await apiClient.download(endpoint, filename)
        toast.success(t('export.downloaded'))
        return true
      } catch (err) {
        // 400/409 on the BAP path mean a precondition failed (non-media source,
        // or no completed Q&A summary — including a hash mismatch after a
        // speaker rename) rather than a generic export failure.
        const isBapPrecondition =
          isBap && err instanceof ApiError && (err.status === 400 || err.status === 409)
        toast.error(t(isBapPrecondition ? 'export.bapError' : 'export.failed'))
        return false
      } finally {
        inFlight.current = false
        setPendingKey(null)
      }
    },
    [t],
  )

  return { pendingKey, isExporting: pendingKey !== null, runExport }
}
