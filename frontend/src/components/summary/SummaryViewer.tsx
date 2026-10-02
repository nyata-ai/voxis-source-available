import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { AlertCircle, Copy, Check, Loader2, Quote } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { cn, formatDuration } from '@/lib/utils'
import { getStructuredSummaryContent, parseQuestionAnswerContent } from '@/lib/summary-content'
import { getSummaryTypeMeta } from './SummaryTypeMeta'
import type {
  StructuredActionItemsSummary,
  StructuredGeneralSummary,
  StructuredKeyPointsSummary,
  StructuredQuestionAndAnswerSummary,
  SummaryCitation,
  SummaryDetail,
} from '@/types/summary'

// Resolves a `summary` namespace key to its localized string.
type TranslateFn = (key: string, options?: Record<string, unknown>) => string

interface SummaryViewerProps {
  /** Absent while the caller is still fetching, or when nothing was generated. */
  summary?: SummaryDetail
  /** Renders a loading line instead of any content or status branch. */
  isLoading?: boolean
  /** Drops the Card chrome and the type heading — for hosts that already supply
   *  both (the briefing pane's tabs name the type). Copy and `actions` stay. */
  headless?: boolean
  /** Controls rendered alongside Copy, e.g. a per-lens Regenerate button. */
  actions?: ReactNode
  /** Lets an enclosing reader seek its transcript when a citation has a timestamp. */
  onCitationSeek?: (startSeconds: number) => void
}

/** The general lens's prose column. The reader's briefing panel already caps the
 *  band at 78ch (artboard); a second 65ch cap inside it would render the body
 *  narrower than the design. The standalone card has no such frame, so it keeps
 *  `max-w-prose`. */
function generalBodyClass(headless?: boolean): string {
  return headless ? 'space-y-4' : 'max-w-prose space-y-4'
}

function renderGeneralContent(content: string, bodyClass: string) {
  const paragraphs = content.split('\n\n').filter(Boolean)
  return (
    <div className={bodyClass}>
      {paragraphs.map((paragraph, i) => (
        <p key={i} className="text-base leading-7">
          {paragraph}
        </p>
      ))}
    </div>
  )
}

interface ParsedKeyPoint {
  point: string
  evidence?: string
}

function citationLabel(citation: SummaryCitation | undefined, id: string, t: TranslateFn) {
  if (!citation) return t('viewer.citation', { id })
  const start = citation.start_seconds
  const time =
    typeof start === 'number' && Number.isFinite(start) && start >= 0 ? formatDuration(start) : null
  if (citation.speaker && time)
    return t('viewer.citationWithSpeakerTime', { id, speaker: citation.speaker, time })
  if (citation.speaker) return t('viewer.citationWithSpeaker', { id, speaker: citation.speaker })
  if (time) return t('viewer.citationWithTime', { id, time })
  return t('viewer.citation', { id })
}

/** True when the structured body this summary renders actually carries chips —
 *  the only case where the Show sources toggle has anything to reveal. */
function hasStructuredCitations(summary: SummaryDetail): boolean {
  const structured = getStructuredSummaryContent(summary)
  if (!structured) return false
  const matchesShape =
    summary.summary_type === 'general' ? 'paragraphs' in structured : 'items' in structured
  if (!matchesShape) return false
  const entries = 'paragraphs' in structured ? structured.paragraphs : structured.items
  return entries.some((entry) => entry.citation_ids.length > 0)
}

function CitationChips({
  show,
  citationIDs,
  citations,
  onCitationSeek,
  t,
}: {
  show: boolean
  citationIDs: string[]
  citations?: SummaryCitation[]
  onCitationSeek?: (startSeconds: number) => void
  t: TranslateFn
}) {
  if (!show || citationIDs.length === 0) return null
  const citationsByID = new Map(citations?.map((citation) => [citation.id, citation]))

  return (
    <div className="mt-2 flex flex-wrap gap-1.5" aria-label={t('viewer.citations')}>
      {citationIDs.map((id) => {
        const citation = citationsByID.get(id)
        const start = citation?.start_seconds
        const canSeek =
          onCitationSeek && typeof start === 'number' && Number.isFinite(start) && start >= 0
        const className =
          'rounded-full border border-border bg-muted px-2 py-0.5 text-xs text-muted-foreground'
        const label = citationLabel(citation, id, t)

        if (canSeek) {
          return (
            <button
              key={id}
              type="button"
              className={className}
              onClick={() => onCitationSeek(start)}
            >
              {label}
            </button>
          )
        }
        return (
          <span key={id} className={className}>
            {label}
          </span>
        )
      })}
    </div>
  )
}

function renderStructuredContent(
  summary: SummaryDetail,
  t: TranslateFn,
  showSources: boolean,
  bodyClass: string,
  onCitationSeek?: (startSeconds: number) => void
) {
  const structured = getStructuredSummaryContent(summary)
  if (!structured) return null

  if (summary.summary_type === 'general' && 'paragraphs' in structured) {
    const content = structured as StructuredGeneralSummary
    return (
      <div className={bodyClass}>
        {content.paragraphs.map((paragraph) => (
          <div key={paragraph.id}>
            <p className="text-base leading-7">{paragraph.text}</p>
            <CitationChips
              show={showSources}
              citationIDs={paragraph.citation_ids}
              citations={summary.citations}
              onCitationSeek={onCitationSeek}
              t={t}
            />
          </div>
        ))}
      </div>
    )
  }

  if (summary.summary_type === 'key_points' && 'items' in structured) {
    const content = structured as StructuredKeyPointsSummary
    return (
      <div className="space-y-3" role="list">
        {content.items.map((item) => (
          <div key={item.id} className="flex gap-3 text-base leading-7" role="listitem">
            <span
              className="mt-[0.6rem] h-1.5 w-1.5 shrink-0 rounded-full bg-foreground"
              aria-hidden="true"
            />
            <div className="min-w-0">
              <p>{item.text}</p>
              <CitationChips
                show={showSources}
                citationIDs={item.citation_ids}
                citations={summary.citations}
                onCitationSeek={onCitationSeek}
                t={t}
              />
            </div>
          </div>
        ))}
      </div>
    )
  }

  if (summary.summary_type === 'action_items' && 'items' in structured) {
    const content = structured as StructuredActionItemsSummary
    return (
      <div className="space-y-3" role="list">
        {content.items.map((item, index) => (
          <div key={item.id} className="rounded-lg border p-4 text-base leading-7" role="listitem">
            <div className="flex items-start gap-2">
              <span className="font-medium text-muted-foreground">{index + 1}.</span>
              <div className="min-w-0 flex-1">
                <div className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {t(`viewer.actionKinds.${item.kind}`)}
                </div>
                <p className="break-words">{item.text}</p>
                <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
                  <span>
                    {t('viewer.owner')} {item.owner ?? t('viewer.notSpecified')}
                  </span>
                  <span>
                    {t('viewer.due')} {item.deadline ?? t('viewer.notSpecified')}
                  </span>
                </div>
                <CitationChips
                  show={showSources}
                  citationIDs={item.citation_ids}
                  citations={summary.citations}
                  onCitationSeek={onCitationSeek}
                  t={t}
                />
              </div>
            </div>
          </div>
        ))}
      </div>
    )
  }

  if (summary.summary_type === 'q_and_a' && 'items' in structured) {
    const content = structured as StructuredQuestionAndAnswerSummary
    return (
      <div className="space-y-4" role="list">
        {content.items.map((item) => (
          <div key={item.id} className="space-y-3 rounded-lg border p-4" role="listitem">
            <div className="flex gap-3 text-base leading-7">
              <span className="font-semibold text-primary" aria-hidden="true">
                {t('viewer.questionMarker')}
              </span>
              <span className="sr-only">{t('viewer.questionPrefix')}</span>
              <p className="min-w-0 whitespace-pre-wrap break-words">
                {item.question_speaker && `[${item.question_speaker}] `}
                {item.question}
              </p>
            </div>
            <div className="flex gap-3 text-base leading-7">
              <span className="font-semibold text-muted-foreground" aria-hidden="true">
                {t('viewer.answerMarker')}
              </span>
              <span className="sr-only">{t('viewer.answerPrefix')}</span>
              <p className="min-w-0 whitespace-pre-wrap break-words">
                {item.answer_speaker && `[${item.answer_speaker}] `}
                {item.answer}
              </p>
            </div>
            <CitationChips
              show={showSources}
              citationIDs={item.citation_ids}
              citations={summary.citations}
              onCitationSeek={onCitationSeek}
              t={t}
            />
          </div>
        ))}
      </div>
    )
  }

  return null
}

function parseKeyPoint(line: string): ParsedKeyPoint {
  const text = line.replace(/^[-*•–—]\s+/, '')
  const delimiter = ' Evidence: '
  const delimiterIndex = text.lastIndexOf(delimiter)
  if (delimiterIndex < 0) return { point: text }
  const point = text.slice(0, delimiterIndex).trim()
  const evidence = text.slice(delimiterIndex + delimiter.length).trim()
  if (!point || !evidence) return { point: text }
  return {
    point,
    evidence: evidence.replace(/^"|"\.?$/g, '').trim(),
  }
}

function hasDegradationCodes(summary: SummaryDetail): boolean {
  const codes = summary.generation_metadata?.degradation_codes
  return (
    Array.isArray(codes) &&
    codes.length > 0 &&
    codes.every((code) => typeof code === 'string' && code.trim().length > 0)
  )
}

function renderKeyPoints(content: string, t: TranslateFn) {
  const lines = content
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
  const items = lines.map(parseKeyPoint)
  return (
    <div className="space-y-3" role="list">
      {items.map((item, i) => (
        <div key={i} className="flex gap-3 text-base leading-7" role="listitem">
          <span
            className="mt-[0.6rem] h-1.5 w-1.5 shrink-0 rounded-full bg-foreground"
            aria-hidden="true"
          />
          <span className="min-w-0">
            <span>{item.point}</span>
            {item.evidence && (
              <span className="mt-1 block text-sm leading-6 text-muted-foreground">
                {t('viewer.evidence')} {item.evidence}
              </span>
            )}
          </span>
        </div>
      ))}
    </div>
  )
}

interface ParsedActionItem {
  raw: string
  type?: string
  owner?: string
  due?: string
  item?: string
  evidence?: string
}

function cleanActionValue(value: string) {
  return value
    .trim()
    .replace(/^"|"\.?$/g, '')
    .trim()
}

function parseActionItem(line: string): ParsedActionItem {
  const raw = line.replace(/^\d+[.)]\s+/, '')
  const parsed: ParsedActionItem = { raw }

  for (const field of raw.split(';')) {
    const separator = field.indexOf(':')
    if (separator < 0) continue

    const key = field.slice(0, separator).trim().toLowerCase()
    const value = cleanActionValue(field.slice(separator + 1))
    if (!value) continue

    if (key === 'type') parsed.type = value
    if (key === 'owner') parsed.owner = value
    if (key === 'due') parsed.due = value
    if (key === 'item') parsed.item = value
    if (key === 'evidence') parsed.evidence = value
  }

  return parsed.item ? parsed : { raw }
}

function renderActionItems(content: string, t: TranslateFn) {
  const lines = content
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
  const items = lines.map((line, i) => ({
    number: i + 1,
    action: parseActionItem(line),
  }))
  return (
    <div className="space-y-3" role="list">
      {items.map((item) => (
        <div
          key={item.number}
          className="rounded-lg border p-4 text-base leading-7"
          role="listitem"
        >
          <div className="flex items-start gap-2">
            <span className="font-medium text-muted-foreground">{item.number}.</span>
            <div className="min-w-0 flex-1">
              {item.action.type && (
                <div className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {item.action.type}
                </div>
              )}
              <p className="break-words">{item.action.item ?? item.action.raw}</p>
              {(item.action.owner || item.action.due) && (
                <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
                  {item.action.owner && (
                    <span>
                      {t('viewer.owner')} {item.action.owner}
                    </span>
                  )}
                  {item.action.due && (
                    <span>
                      {t('viewer.due')} {item.action.due}
                    </span>
                  )}
                </div>
              )}
              {item.action.evidence && (
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                  {t('viewer.evidence')} {item.action.evidence}
                </p>
              )}
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function renderQuestionAnswer(content: string, t: TranslateFn, bodyClass: string) {
  const items = parseQuestionAnswerContent(content)
  if (!items) return renderGeneralContent(content, bodyClass)

  return (
    <div className="space-y-4" role="list">
      {items.map((item, i) => (
        <div key={i} className="space-y-3 rounded-lg border p-4" role="listitem">
          <div className="flex gap-3 text-base leading-7">
            <span className="font-semibold text-primary" aria-hidden="true">
              {t('viewer.questionMarker')}
            </span>
            <span className="sr-only">{t('viewer.questionPrefix')}</span>
            <p className="min-w-0 whitespace-pre-wrap break-words">{item.question}</p>
          </div>
          <div className="flex gap-3 text-base leading-7">
            <span className="font-semibold text-muted-foreground" aria-hidden="true">
              {t('viewer.answerMarker')}
            </span>
            <span className="sr-only">{t('viewer.answerPrefix')}</span>
            <p className="min-w-0 whitespace-pre-wrap break-words">{item.answer}</p>
          </div>
        </div>
      ))}
    </div>
  )
}

function renderContent(
  summary: SummaryDetail,
  t: TranslateFn,
  showSources: boolean,
  bodyClass: string,
  onCitationSeek?: (startSeconds: number) => void
) {
  const structured = renderStructuredContent(summary, t, showSources, bodyClass, onCitationSeek)
  const content =
    structured ??
    (() => {
      if (!summary.content) {
        return <p className="text-sm text-muted-foreground">{t('viewer.noContent')}</p>
      }

      switch (summary.summary_type) {
        case 'key_points':
          return renderKeyPoints(summary.content, t)
        case 'action_items':
          return renderActionItems(summary.content, t)
        case 'q_and_a':
          return renderQuestionAnswer(summary.content, t, bodyClass)
        case 'general':
        default:
          return renderGeneralContent(summary.content, bodyClass)
      }
    })()

  return (
    <>
      {hasDegradationCodes(summary) && (
        <p
          role="status"
          className="rounded-md border border-warning/40 bg-warning/10 px-3 py-2 text-sm"
        >
          {t('viewer.degradationNotice')}
        </p>
      )}
      {content}
    </>
  )
}

// A status branch's body, wrapped in the Card chrome unless the host is
// supplying its own frame. `actions` rides along on every branch — a failed
// briefing is exactly where its host's Regenerate control is needed.
function StatusFrame({
  headless,
  className,
  actions,
  children,
}: {
  headless?: boolean
  className: string
  actions?: ReactNode
  children: ReactNode
}) {
  const body = (
    <>
      {actions && <div className="flex flex-row items-center justify-end gap-2">{actions}</div>}
      <div className={className}>{children}</div>
    </>
  )
  if (headless) return <div className="space-y-3">{body}</div>
  return (
    <Card>
      <CardContent className="pt-6">{body}</CardContent>
    </Card>
  )
}

export function SummaryViewer({
  summary,
  isLoading,
  headless,
  actions,
  onCitationSeek,
}: SummaryViewerProps) {
  const { t } = useTranslation('summary')
  const [copied, setCopied] = useState(false)
  // Off by default — the reading view stays clean until the reader asks for
  // provenance. Deliberately not persisted: it is a per-view glance, not a setting.
  const [showSources, setShowSources] = useState(false)

  async function handleCopy() {
    if (!summary?.content) return
    try {
      await navigator.clipboard.writeText(summary.content)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard access denied — silently ignore
    }
  }

  // Loading wins over every other branch: a stale summary from the previously
  // selected lens must not flash under the new lens's tab.
  if (isLoading) {
    return (
      <StatusFrame
        headless={headless}
        actions={actions}
        className="flex items-center justify-center gap-2 py-12"
      >
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        <span className="text-sm text-muted-foreground">{t('viewer.loading')}</span>
      </StatusFrame>
    )
  }

  if (!summary) {
    return (
      <StatusFrame headless={headless} actions={actions} className="py-8">
        <p className="text-sm text-muted-foreground">{t('viewer.noContent')}</p>
      </StatusFrame>
    )
  }

  if (summary.status === 'pending') {
    return (
      <StatusFrame
        headless={headless}
        actions={actions}
        className="flex items-center justify-center gap-2 py-12"
      >
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        <span className="text-sm text-muted-foreground">{t('viewer.processing')}</span>
      </StatusFrame>
    )
  }

  if (summary.status === 'failed') {
    // Inside the reader's briefing band the artboard specifies coral for the
    // failed lens; the standalone card keeps the app's destructive red.
    const failTone = headless ? 'text-coral' : 'text-destructive'
    return (
      <StatusFrame headless={headless} actions={actions} className="flex items-center gap-2 py-8">
        <AlertCircle className={cn('h-5 w-5', failTone)} />
        <span className={cn('text-sm', failTone)}>
          {summary.error_message ?? t('viewer.unknownError')}
        </span>
      </StatusFrame>
    )
  }

  const canShowSources = hasStructuredCitations(summary)
  const hasControls = Boolean(actions) || canShowSources || Boolean(summary.content)
  const controls = (
    <>
      {actions}
      {canShowSources && (
        <Button
          variant="ghost"
          size="sm"
          className="min-h-11 sm:min-h-9"
          onClick={() => setShowSources((previous) => !previous)}
          aria-pressed={showSources}
        >
          <Quote className="mr-1 h-4 w-4" />
          {showSources ? t('viewer.hideSources') : t('viewer.showSources')}
        </Button>
      )}
      {summary.content && (
        <Button
          variant="ghost"
          size="sm"
          className="min-h-11 sm:min-h-9"
          onClick={handleCopy}
          aria-label={copied ? t('viewer.copied') : t('viewer.copy')}
        >
          {copied ? (
            <>
              <Check className="mr-1 h-4 w-4" />
              {t('viewer.copied')}
            </>
          ) : (
            <>
              <Copy className="mr-1 h-4 w-4" />
              {t('viewer.copy')}
            </>
          )}
        </Button>
      )}
    </>
  )

  if (headless) {
    return (
      <div className="space-y-3">
        {hasControls && (
          <div className="flex flex-row items-center justify-end gap-2">{controls}</div>
        )}
        {renderContent(summary, t, showSources, generalBodyClass(true), onCitationSeek)}
      </div>
    )
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-3">
        <CardTitle className="text-base font-medium">
          {t(getSummaryTypeMeta(summary.summary_type).labelKey)}
        </CardTitle>
        {hasControls && <div className="flex flex-row items-center gap-2">{controls}</div>}
      </CardHeader>
      <CardContent>
        {renderContent(summary, t, showSources, generalBodyClass(false), onCitationSeek)}
      </CardContent>
    </Card>
  )
}
