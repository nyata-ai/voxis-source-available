import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { FileAudio, MoreHorizontal, Pencil, RefreshCw, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { StatusChip } from '@/components/ui/status-chip'
import { LanguageBadge } from './LanguageBadge'
import { statusTone } from '@/lib/status-tone'
import { formatDuration, cn } from '@/lib/utils'
import { formatLanguageCodes } from '@/lib/languages'
import { useFormatters } from '@/i18n/useFormatters'
export interface SessionRowItem {
  key: string
  title: string
  to: string
  status: string
  languages: string[]
  retranscribable: boolean
  lensCount?: number
  durationSeconds?: number
  speakerCount?: number
  wordCount?: number
  dateValue?: string
}

export interface SessionRowProps {
  item: SessionRowItem
  /**
   * Caller must gate applicability per item (e.g. only pass this for rows
   * the user is allowed to delete) — SessionRow shows the overflow menu
   * whenever a handler is present, with no eligibility check of its own.
   */
  onDelete?: (item: SessionRowItem) => void
  /**
   * Unlike the other handlers, eligibility travels with the row: the menu
   * item renders only when `item.retranscribable` is also true, so callers
   * may pass this unconditionally.
   */
  onRetranscribe?: (item: SessionRowItem) => void
  /** Caller must gate applicability per item before exposing rename. */
  onRename?: (item: SessionRowItem) => void
}

/**
 * One row for each transcription session, built entirely from a LibraryItem. A fixed-height
 * (44px) single-line grid: source glyph, title + meta, then language/lens/
 * status indicators and an optional overflow menu.
 */
export function SessionRow({ item, onDelete, onRetranscribe, onRename }: SessionRowProps) {
  const { t } = useTranslation(['common', 'transcription'])
  const formatters = useFormatters()

  const SourceIcon = FileAudio
  const sourceLabel = t('common:session.source.audio')
  const title = item.title.trim() || t('common:session.untitled')

  const meta = [
    item.speakerCount && item.speakerCount > 0
      ? t('common:session.speaker', { count: item.speakerCount })
      : null,
    item.wordCount && item.wordCount > 0
      ? t('common:session.words', { n: formatters.integer(item.wordCount) })
      : null,
    item.dateValue ? formatters.relativeDate(item.dateValue) : null,
  ]
    .filter(Boolean)
    .join(' · ')

  const showLanguageBadge = item.languages.length > 0 && !item.languages.includes('auto')
  const lensCount = item.lensCount ?? 0
  const showLensChip = lensCount > 0
  const tone = statusTone(item.status)
  const showStatusChip = tone !== 'neutral'
  const showDuration =
    !showStatusChip && item.durationSeconds !== undefined && item.durationSeconds > 0
  const showDropdown = !!onDelete || !!onRename || (!!onRetranscribe && item.retranscribable)

  return (
    <div className="group/row relative h-11 border-b border-border/60 last:border-b-0">
      <Link
        to={item.to}
        className={cn(
          'grid h-11 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 px-3',
          'transition-colors hover:bg-accent/30'
        )}
      >
        <span className="flex shrink-0 items-center gap-1" aria-hidden="true">
          <SourceIcon className="h-4 w-4 text-muted-foreground" />
        </span>

        <span className="min-w-0">
          <span className="sr-only">{sourceLabel}.</span>
          <span className="block truncate text-sm font-medium text-foreground">{title}</span>
          {meta && (
            <span className="block truncate text-[0.7rem] text-muted-foreground">{meta}</span>
          )}
        </span>

        <span className="flex shrink-0 items-center gap-2 pr-7">
          {showLanguageBadge && (
            <span
              title={t('common:session.languages', {
                codes: formatLanguageCodes(item.languages),
              })}
            >
              <LanguageBadge languages={item.languages} />
            </span>
          )}
          {showLensChip && (
            <span className="text-[0.68rem] text-muted-foreground">
              {t('common:session.lens', { count: lensCount })}
            </span>
          )}
          {showStatusChip && <StatusChip status={item.status} />}
          {showDuration && (
            <span className="text-[0.7rem] text-muted-foreground">
              {formatDuration(item.durationSeconds ?? 0)}
            </span>
          )}
        </span>
      </Link>

      {showDropdown && (
        <div className="pointer-events-none absolute right-1.5 top-1/2 -translate-y-1/2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="pointer-events-auto h-7 w-7 shrink-0"
                aria-label={t('transcription:row.actionsLabel', { title })}
                // preventDefault + stopPropagation guards against the phantom
                // click iOS Safari can synthesize on a touch that drifts off
                // the button onto the parent anchor. Same guard as
                // TranscriptionRow's overflow trigger.
                onClick={(e) => {
                  e.preventDefault()
                  e.stopPropagation()
                }}
              >
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
              {onRetranscribe && item.retranscribable && (
                <DropdownMenuItem
                  onClick={(e) => {
                    e.stopPropagation()
                    onRetranscribe(item)
                  }}
                >
                  <RefreshCw className="mr-2 h-4 w-4" />
                  {t('transcription:row.retranscribe')}
                </DropdownMenuItem>
              )}
              {onRename && (
                <DropdownMenuItem
                  onClick={(e) => {
                    e.stopPropagation()
                    onRename(item)
                  }}
                >
                  <Pencil className="mr-2 h-4 w-4" />
                  {t('transcription:row.rename')}
                </DropdownMenuItem>
              )}
              {onDelete && (
                <DropdownMenuItem
                  className="text-destructive focus:text-destructive"
                  onClick={(e) => {
                    e.stopPropagation()
                    onDelete(item)
                  }}
                >
                  <Trash2 className="mr-2 h-4 w-4" />
                  {t('transcription:row.delete')}
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      )}
    </div>
  )
}
