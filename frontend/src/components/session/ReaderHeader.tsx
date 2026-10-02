import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ArrowLeft, Lock, MoreHorizontal, RefreshCw, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Chip, CHIP_CLASS } from '@/components/ui/chip'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { StatusChip } from '@/components/ui/status-chip'
import { InlineEditableText } from '@/components/media/InlineEditableText'
import { useFormatters } from '@/i18n/useFormatters'
import { statusTone } from '@/lib/status-tone'
import { cn, formatBytes, formatDuration } from '@/lib/utils'
import { LanguageBadge } from './LanguageBadge'
import type { SessionView } from './session-view'

/** The file extension of a stored filename, upper-cased — "MP3", "WAV". Absent
 *  when the name carries none, in which case the source line simply omits it
 *  rather than guessing a container from the MIME type. */
function fileFormat(filename: string): string | null {
  const match = /\.([a-z0-9]{2,5})$/i.exec(filename.trim())
  return match ? match[1].toUpperCase() : null
}

/** The facts a session actually has, as chips. Every one is conditional: a chip
 *  reading "0 speakers" is worse than no chip. */
function ReaderChips({ session }: { session: SessionView }) {
  const { t } = useTranslation(['transcription', 'common', 'media'])
  const formatters = useFormatters()

  return (
    <div data-testid="reader-meta" className="flex flex-wrap items-center gap-2">
      {statusTone(session.status) !== 'neutral' && <StatusChip status={session.status} />}
      <LanguageBadge languages={session.languages} className={CHIP_CLASS} />
      {session.durationSeconds > 0 && <Chip>{formatDuration(session.durationSeconds)}</Chip>}
      {session.speakerCount > 0 && (
        <Chip>{t('common:session.speaker', { count: session.speakerCount })}</Chip>
      )}
      {session.wordCount > 0 && (
        <Chip>{t('common:session.words', { n: formatters.integer(session.wordCount) })}</Chip>
      )}
      {session.encryptionAlgo && (
        <Chip>
          <Lock className="h-3 w-3" aria-hidden="true" />
          {t('media:detail.encrypted')}
        </Chip>
      )}
      {session.preprocessorUsed && <Chip>{t('transcription:detail.enhanced')}</Chip>}
    </div>
  )
}

/** Where the uploaded session came from. */
function ReaderSource({ session }: { session: SessionView }) {
  const { t } = useTranslation(['transcription'])
  const formatters = useFormatters()

  if (!session.createdAt) return null
  const parts = [
    t('transcription:reader.uploadedLine', { date: formatters.date(session.createdAt) }),
    session.sizeBytes ? formatBytes(session.sizeBytes) : null,
    fileFormat(session.titlePlaceholder),
  ].filter(Boolean)

  return (
    <p className="text-[13px] leading-normal text-ink-muted dark:text-muted-foreground">
      {parts.join(' · ')}
    </p>
  )
}

/** Retranscribe and Delete: the two rare, consequential actions. Both keep the
 *  confirmation dialog they always had — the menu only moves them out of the
 *  way of the ones a reader uses every visit. */
function ReaderMoreMenu({
  session,
  onDelete,
  onRetranscribe,
}: {
  session: SessionView
  onDelete: () => void
  onRetranscribe: () => void
}) {
  const { t } = useTranslation(['transcription'])
  const canRetranscribe = session.can.retranscribe && Boolean(session.retranscribeSource)
  const canDelete = session.can.delete && Boolean(session.actions.delete)
  if (!canRetranscribe && !canDelete) return null

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="h-10 w-10 shrink-0 rounded-[10px]"
          aria-label={t('transcription:reader.more')}
        >
          <MoreHorizontal className="h-5 w-5" aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {canRetranscribe && (
          <DropdownMenuItem onSelect={onRetranscribe}>
            <RefreshCw className="mr-2 h-4 w-4" aria-hidden="true" />
            {t('transcription:reader.retranscribe')}
          </DropdownMenuItem>
        )}
        {canDelete && (
          <DropdownMenuItem onSelect={onDelete} className="text-destructive focus:text-destructive">
            <Trash2 className="mr-2 h-4 w-4" aria-hidden="true" />
            {t('transcription:detail.delete')}
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export interface ReaderHeaderProps {
  session: SessionView
  onDelete: () => void
  onRetranscribe: () => void
}

/**
 * The reader's identity block: the way back, the title, what the session is
 * made of, where it came from, and its description. Export and Download Audio
 * live in the export panel now; the only actions left here are the two rare
 * ones, folded into a More menu.
 */
export function ReaderHeader({ session, onDelete, onRetranscribe }: ReaderHeaderProps) {
  const { t } = useTranslation(['transcription'])
  const titleClass = 'text-[28px] font-normal leading-[1.2]'

  return (
    <header className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-4">
        <Link
          to="/library"
          aria-label={t('transcription:reader.backLabel')}
          className="inline-flex min-h-11 items-center gap-2 text-[13px] leading-normal text-ink-muted hover:text-coral dark:text-muted-foreground sm:min-h-8"
        >
          <ArrowLeft className="h-4 w-4" aria-hidden="true" />
          {t('transcription:reader.library')}
        </Link>
        <ReaderMoreMenu session={session} onDelete={onDelete} onRetranscribe={onRetranscribe} />
      </div>

      <h1 className={cn(titleClass, 'max-w-[32ch]')}>
        {session.can.editTitle ? (
          <InlineEditableText
            value={session.title}
            placeholder={session.titlePlaceholder}
            ariaLabel={t('transcription:detail.titleLabel')}
            onSave={session.actions.saveTitle}
            as="span"
            maxLength={255}
            className={titleClass}
            inputClassName={titleClass}
          />
        ) : (
          <span>{session.title || session.titlePlaceholder}</span>
        )}
      </h1>

      <ReaderChips session={session} />

      <ReaderSource session={session} />

      {/* A div, not a p: the multiline editor renders a wrapper div. */}
      <div className="max-w-[720px] text-[15px] leading-[1.65] text-ink-body dark:text-foreground">
        {session.can.editDescription ? (
          <InlineEditableText
            value={session.description}
            placeholder={t('transcription:detail.descriptionPlaceholder')}
            ariaLabel={t('transcription:detail.descriptionLabel')}
            onSave={session.actions.saveDescription}
            multiline
            maxLength={1000}
            as="span"
            className="text-[15px] leading-[1.65]"
            inputClassName="text-[15px]"
          />
        ) : (
          <span>{session.description}</span>
        )}
      </div>
    </header>
  )
}
