import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ArrowLeft, Lock, MoreHorizontal, Pencil, RefreshCw, Trash2 } from 'lucide-react'
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
import { SpeakerDisc } from '@/components/transcription/SpeakerDisc'
import { useFormatters } from '@/i18n/useFormatters'
import { statusTone } from '@/lib/status-tone'
import { cn, formatBytes, formatDuration } from '@/lib/utils'
import { LanguageBadge } from './LanguageBadge'
import { ReaderSectionLinks, type ReaderSection } from './ReaderSectionLinks'
import type { SessionView } from './session-view'

/** The file extension of a stored filename, upper-cased — "MP3", "WAV". Absent
 *  when the name carries none, in which case the source line simply omits it
 *  rather than guessing a container from the MIME type. */
function fileFormat(filename: string): string | null {
  const match = /\.([a-z0-9]{2,5})$/i.exec(filename.trim())
  return match ? match[1].toUpperCase() : null
}

/** Discs shown in the speaker chip: the first five, one per palette colour. */
const CHIP_DISCS = 5

/** The speaker count chip, led by a stack of the speakers' colour discs so
 *  the colours read as a key before the transcript starts. */
function SpeakerChip({ count }: { count: number }) {
  const { t } = useTranslation(['common'])
  const discs = Math.min(count, CHIP_DISCS)
  return (
    <Chip>
      <span className="flex -space-x-1" aria-hidden="true">
        {Array.from({ length: discs }, (_, index) => (
          <SpeakerDisc
            key={index}
            speakerIndex={index}
            size="xs"
            className="ring-1 ring-paper-soft dark:ring-muted"
          />
        ))}
      </span>
      {t('common:session.speaker', { count })}
    </Chip>
  )
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
      {session.speakerCount > 0 && <SpeakerChip count={session.speakerCount} />}
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
    <p className="text-body text-ink-muted dark:text-muted-foreground">
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
  onAddDescription,
}: {
  session: SessionView
  onDelete: () => void
  onRetranscribe: () => void
  /** Present only while the session has no description to click on. */
  onAddDescription?: () => void
}) {
  const { t } = useTranslation(['transcription'])
  const canRetranscribe = session.can.retranscribe && Boolean(session.retranscribeSource)
  const canDelete = session.can.delete && Boolean(session.actions.delete)
  // Closing the menu hands focus back to its trigger, which would blur (and so
  // close) the description editor the Add item just opened.
  const keepFocus = useRef(false)
  if (!canRetranscribe && !canDelete && !onAddDescription) return null

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="h-11 w-11 shrink-0 rounded-[10px] lg:h-9 lg:w-9"
          aria-label={t('transcription:reader.more')}
        >
          <MoreHorizontal className="h-5 w-5" aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        onCloseAutoFocus={(event) => {
          if (!keepFocus.current) return
          keepFocus.current = false
          event.preventDefault()
        }}
      >
        {onAddDescription && (
          <DropdownMenuItem
            onSelect={() => {
              keepFocus.current = true
              onAddDescription()
            }}
          >
            <Pencil className="mr-2 h-4 w-4" aria-hidden="true" />
            {t('transcription:detail.descriptionPlaceholder')}
          </DropdownMenuItem>
        )}
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
  /** In-page sections for the bar's links; fewer than two draws none. */
  sections?: ReaderSection[]
}

/** The editable description, shown once there is one to read or the reader
 *  asked to add one from the More menu (`autoEdit` then opens the editor). */
function ReaderDescription({ session, autoEdit }: { session: SessionView; autoEdit: boolean }) {
  const { t } = useTranslation(['transcription'])
  return (
    // A div, not a p: the multiline editor renders a wrapper div.
    <div className="max-w-[720px] text-reading text-ink-body dark:text-foreground">
      {session.can.editDescription ? (
        <InlineEditableText
          value={session.description}
          placeholder={t('transcription:detail.descriptionPlaceholder')}
          ariaLabel={t('transcription:detail.descriptionLabel')}
          onSave={session.actions.saveDescription}
          multiline
          maxLength={1000}
          as="span"
          className="text-reading"
          inputClassName="text-reading"
          defaultEditing={autoEdit}
        />
      ) : (
        <span>{session.description}</span>
      )}
    </div>
  )
}

/** One line from `sm`, whatever its length: the editable title's button and
 *  text are allowed to shrink so the ellipsis lands inside the heading. A phone
 *  wraps instead, because one line there would cut most titles to a word. */
const TITLE_ONE_LINE =
  'min-w-0 sm:truncate sm:[&>button]:max-w-full sm:[&>button>span]:block sm:[&>button>span]:min-w-0 sm:[&>button>span]:truncate'

/**
 * The reader's identity block, kept short so the transcript starts high.
 *
 * First a sticky bar: the way back, the in-page section links (Transcript ·
 * Briefings · Export) and the More menu. It stays under the app header while
 * the page scrolls, so the briefings below the workspace are always one click
 * away. Then the title on one line and one row of what the session is made of
 * and where it came from. The description shows once there is one; adding it
 * lives in the More menu with the two rare actions, Retranscribe and Delete.
 *
 * Returns two siblings, not one wrapper: a sticky element only sticks inside
 * its parent, so the bar must be a direct child of the long page column.
 */
export function ReaderHeader({
  session,
  onDelete,
  onRetranscribe,
  sections = [],
}: ReaderHeaderProps) {
  const { t } = useTranslation(['transcription'])
  const [addingDescription, setAddingDescription] = useState(false)
  const titleClass = 'text-page font-normal'
  const fullTitle = session.title || session.titlePlaceholder
  const hasDescription = session.description.trim() !== ''
  const showDescription = hasDescription || addingDescription
  const canAddDescription = session.can.editDescription && !showDescription

  return (
    <>
      <div
        data-testid="reader-bar"
        // `mb-3` beats the page's `space-y` gap (a zero-specificity selector),
        // so the title sits close under the bar.
        className="sticky top-[var(--app-header-h)] z-20 mb-3 flex h-[var(--reader-nav-h)] items-center gap-2 border-b border-border bg-background"
      >
        <Link
          to="/library"
          aria-label={t('transcription:reader.backLabel')}
          className="inline-flex min-h-11 shrink-0 items-center gap-2 rounded-md text-body text-ink-muted hover:text-coral-mark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:text-muted-foreground lg:min-h-9"
        >
          <ArrowLeft className="h-4 w-4" aria-hidden="true" />
          {/* On a phone the arrow alone (the link keeps its name) leaves the
              bar room for the section links. */}
          <span className="hidden sm:inline">{t('transcription:reader.library')}</span>
        </Link>
        <ReaderSectionLinks sections={sections} />
        <span className="ml-auto shrink-0">
          <ReaderMoreMenu
            session={session}
            onDelete={onDelete}
            onRetranscribe={onRetranscribe}
            onAddDescription={canAddDescription ? () => setAddingDescription(true) : undefined}
          />
        </span>
      </div>

      <header className="flex flex-col gap-2">
        <h1 className={cn(titleClass, TITLE_ONE_LINE)} title={fullTitle}>
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
            <span>{fullTitle}</span>
          )}
        </h1>

        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <ReaderChips session={session} />
          <ReaderSource session={session} />
        </div>

        {showDescription && <ReaderDescription session={session} autoEdit={addingDescription} />}
      </header>
    </>
  )
}
