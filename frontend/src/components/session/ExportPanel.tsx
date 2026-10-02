import { useId, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { PILL_CLASS } from '@/components/ui/chip'
import { Panel, PanelTitle, PANEL_ROW_CLASS } from '@/components/ui/panel'
import { DownloadAudioButton } from '@/components/media/DownloadAudioButton'
import { getSummaryTypeMeta } from '@/components/summary/SummaryTypeMeta'
import {
  EXPORT_FORMATS,
  useExportRequest,
  type ExportChoice,
  type ExportEntityType,
  type ExportRunner,
} from '@/components/export/useExportRequest'
import { cn } from '@/lib/utils'
import type { SummaryType } from '@/types/summary'
import type { SessionView } from './session-view'

/**
 * The shared interactive pill plus this panel's size and its two "cannot run"
 * looks. They are deliberately different states: `aria-disabled` marks an
 * export that is not available yet — the button stays focusable so its hint is
 * reachable — while `disabled` is the momentary hold while another export
 * downloads, which needs no explanation.
 */
const FORMAT_PILL = cn(
  PILL_CLASS,
  'h-11 px-3.5 text-[13px] sm:h-8 sm:px-2.5 sm:text-xs',
  'disabled:cursor-not-allowed disabled:opacity-50',
  'aria-disabled:cursor-not-allowed aria-disabled:opacity-50',
  // Cancelling the coral hover takes a per-theme pair: the light theme's resting
  // ink is not the dark theme's, and inheriting only the light one paints
  // near-black text on the dark muted surface.
  'disabled:hover:border-secondary disabled:hover:text-ink-muted',
  'aria-disabled:hover:border-secondary aria-disabled:hover:text-ink-muted',
  'dark:disabled:hover:border-border dark:disabled:hover:text-muted-foreground',
  'dark:aria-disabled:hover:border-border dark:aria-disabled:hover:text-muted-foreground',
)

/** One labelled row of the panel. Stacks on phones, where 44 px pills do not
 *  fit beside their label. */
function ExportRow({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div
      className={cn(
        PANEL_ROW_CLASS,
        'flex flex-col gap-2.5 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between sm:gap-3',
      )}
    >
      <span className="text-sm text-foreground">{label}</span>
      <div className="flex flex-wrap items-center gap-1.5">{children}</div>
    </div>
  )
}

interface FormatPillProps {
  runner: ExportRunner
  choice: ExportChoice
  /** What the pill shows: PDF, DOCX, JSON. */
  label: string
  /** The row it belongs to, so the accessible name says what is exported. */
  rowLabel: string
  entityType: ExportEntityType
  entityId: string
  mediaFilename: string
  /** Present only when the export cannot run yet; also the pill's tooltip. */
  disabledReason?: string
  /** The row's hint element, which this pill describes itself with while it is
   *  blocked. Set only when the row actually renders one. */
  hintId?: string
  /** Identifies this pill in the runner's `pendingKey`. */
  pillKey: string
}

/** One format, one click. Every pill is held while any export is in flight —
 *  two blob downloads at once is not a thing a reader asked for. */
function FormatPill({
  runner,
  choice,
  label,
  rowLabel,
  entityType,
  entityId,
  mediaFilename,
  disabledReason,
  hintId,
  pillKey,
}: FormatPillProps) {
  const { t } = useTranslation('common')
  const busy = runner.pendingKey === pillKey
  const blocked = Boolean(disabledReason) || entityId === ''
  return (
    <button
      type="button"
      className={cn(FORMAT_PILL, busy && 'border-coral text-coral')}
      // `blocked` is aria-disabled and not `disabled`: a disabled button cannot
      // take focus, so neither the tooltip nor the hint below would ever reach
      // a keyboard or screen-reader user. The click is refused instead.
      disabled={runner.isExporting}
      aria-disabled={blocked || undefined}
      aria-busy={busy || undefined}
      aria-label={t('export.pillLabel', { row: rowLabel, format: label })}
      aria-describedby={blocked ? hintId : undefined}
      title={disabledReason}
      onClick={() => {
        if (blocked) return
        void runner.runExport({ entityType, entityId, mediaFilename, choice }, pillKey)
      }}
    >
      {label}
    </button>
  )
}

interface FormatRowProps {
  runner: ExportRunner
  label: string
  entityType: ExportEntityType
  entityId: string
  mediaFilename: string
  /** Why this row cannot export yet, if it cannot. */
  disabledReason?: string
  /** Namespaces this row's pills in the runner's `pendingKey`. */
  keyPrefix: string
}

/** A row of every format, plus the one hint its pills point at. The hint is
 *  rendered here, beside them, so it can never be conditioned out from under
 *  an `aria-describedby` that still names it. */
function FormatRow({
  runner,
  label,
  entityType,
  entityId,
  mediaFilename,
  disabledReason,
  keyPrefix,
}: FormatRowProps) {
  const { t } = useTranslation('common')
  const hintId = useId()
  return (
    <ExportRow label={label}>
      {EXPORT_FORMATS.map((format) => (
        <FormatPill
          key={format}
          runner={runner}
          choice={format}
          label={t(`export.short.${format}`)}
          rowLabel={label}
          entityType={entityType}
          entityId={entityId}
          mediaFilename={mediaFilename}
          disabledReason={disabledReason}
          hintId={disabledReason ? hintId : undefined}
          pillKey={`${keyPrefix}:${format}`}
        />
      ))}
      {disabledReason && (
        <span id={hintId} className="sr-only">
          {disabledReason}
        </span>
      )}
    </ExportRow>
  )
}

export interface ExportPanelProps {
  session: SessionView
  /** The briefing tab currently on screen — the one the Briefing row exports. */
  activeLens: SummaryType
  /** Whether the Berita Acara draft row applies. Derived by `SessionReaderPage`
   *  from the feature flag, the source and the Q&A lens. */
  bapEligible: boolean
}

/**
 * Export as a short list of one-click choices instead of a dialog: the format
 * is the click. The request itself stays in `useExportRequest`, so the endpoint
 * and the fallback filename are derived in exactly one place.
 */
export function ExportPanel({ session, activeLens, bapEligible }: ExportPanelProps) {
  const { t } = useTranslation(['common', 'summary'])
  const runner = useExportRequest()

  const transcriptionId = session.transcriptionId ?? ''
  const canExportTranscript = session.can.export && transcriptionId !== ''
  const lens = session.lenses.find((entry) => entry.type === activeLens)
  const lensReady = lens?.status === 'completed' && Boolean(lens.summaryId)
  // `can.briefings` gates the briefing band itself; a session that may not read
  // its briefings must not be offered a row that exports them either.
  const canExportBriefing =
    session.can.export && session.can.briefings && session.lenses.length > 0
  // `DownloadAudioButton` renders nothing until the row is completed, so the
  // status has to gate the label too — a row reading "Original audio" with no
  // control beside it is worse than no row.
  const canDownloadAudio =
    session.can.downloadAudio && Boolean(session.mediaId) && session.status === 'completed'

  if (!canExportTranscript && !canExportBriefing && !canDownloadAudio) return null

  const transcriptRow = t('common:export.rows.transcript')
  const briefingRow = t('common:export.rows.briefing', {
    lens: t(`summary:${getSummaryTypeMeta(activeLens).labelKey}`),
  })
  const bapRow = t('common:export.rows.bap')

  return (
    <Panel aria-labelledby="export-panel-title" data-testid="reader-export">
      <PanelTitle id="export-panel-title">{t('common:export.panelTitle')}</PanelTitle>

      {canExportTranscript && (
        <FormatRow
          runner={runner}
          label={transcriptRow}
          entityType="transcription"
          entityId={transcriptionId}
          mediaFilename={session.titlePlaceholder}
          keyPrefix="transcript"
        />
      )}

      {canExportBriefing && (
        <FormatRow
          runner={runner}
          label={briefingRow}
          entityType="summary"
          entityId={lens?.summaryId ?? ''}
          mediaFilename={session.titlePlaceholder}
          disabledReason={lensReady ? undefined : t('common:export.briefingUnavailable')}
          keyPrefix="briefing"
        />
      )}

      {bapEligible && canExportTranscript && (
        <ExportRow label={bapRow}>
          <FormatPill
            runner={runner}
            choice="bap"
            label={t('common:export.short.docx')}
            rowLabel={bapRow}
            entityType="transcription"
            entityId={transcriptionId}
            mediaFilename={session.titlePlaceholder}
            pillKey="bap:docx"
          />
        </ExportRow>
      )}

      {canDownloadAudio && session.mediaId && (
        <ExportRow label={t('common:export.rows.audio')}>
          <DownloadAudioButton
            iconOnly
            mediaId={session.mediaId}
            mediaFilename={session.titlePlaceholder}
            status={session.status}
          />
        </ExportRow>
      )}
    </Panel>
  )
}
