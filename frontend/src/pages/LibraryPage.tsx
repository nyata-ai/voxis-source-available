import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ChevronLeft, ChevronRight, FileAudio, FileText, Search } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { useSearchMedia } from '@/hooks/useMedia'
import { useUnifiedSearch } from '@/hooks/useSearch'
import { useTranscriptionList } from '@/hooks/useTranscription'
import type { SearchItem } from '@/types/search'

const PAGE_SIZE = 50

type LibraryRow = {
  key: string
  to: string
  type: string
  title: string
  detail?: string
  status: string
  createdAt: string
  icon: typeof FileAudio
}

export function LibraryPage() {
  const { t } = useTranslation('oss')
  const [search, setSearch] = useState('')
  const [offset, setOffset] = useState(0)
  const [transcriptCursor, setTranscriptCursor] = useState<string>()
  const [transcriptItems, setTranscriptItems] = useState<SearchItem[]>([])
  const completedTranscriptPages = useRef(new Set<string>())
  const query = search.trim()
  const isSearching = query.length > 0
  const media = useSearchMedia({ limit: PAGE_SIZE, offset, enabled: !isSearching })
  const transcriptions = useTranscriptionList(PAGE_SIZE, offset, '', { enabled: !isSearching })
  const transcriptSearch = useUnifiedSearch({
    search: query,
    limit: PAGE_SIZE,
    offset,
    transcriptCursor,
    enabled: isSearching,
  })

  useEffect(() => {
    const page = transcriptSearch.data
    if (!isSearching || !page || page.query !== query) return

    const pageKey = `${query}\u0000${transcriptCursor ?? ''}`
    if (completedTranscriptPages.current.has(pageKey)) return
    completedTranscriptPages.current.add(pageKey)
    setTranscriptItems((current) =>
      transcriptCursor
        ? appendDistinctSearchItems(current, page.transcript_items)
        : page.transcript_items
    )
  }, [isSearching, query, transcriptCursor, transcriptSearch.data])

  const metadataRows = useMemo<LibraryRow[]>(() => {
    if (!isSearching) return []
    return (transcriptSearch.data?.items ?? []).map((item) => ({
      key: `search-${item.id}`,
      to: item.latest_transcription_id
        ? `/transcriptions/${item.latest_transcription_id}`
        : `/media/${item.id}`,
      type: item.match_fields?.includes('transcript')
        ? t('library.transcript')
        : t('library.audio'),
      title: item.title || item.filename || t('library.untitled'),
      detail: item.snippet || item.description,
      status: item.status,
      createdAt: item.created_at,
      icon: item.match_fields?.includes('transcript') ? FileText : FileAudio,
    }))
  }, [isSearching, t, transcriptSearch.data?.items])

  const transcriptRows = useMemo<LibraryRow[]>(
    () =>
      transcriptItems.map((item) => ({
        key: `transcript-search-${transcriptResultKey(item)}`,
        to: item.latest_transcription_id
          ? `/transcriptions/${item.latest_transcription_id}`
          : `/media/${item.id}`,
        type: t('library.transcript'),
        title: item.title || item.filename || t('library.untitled'),
        detail: item.snippet || item.description,
        status: item.status,
        createdAt: item.created_at,
        icon: FileText,
      })),
    [t, transcriptItems]
  )

  const rows = useMemo<LibraryRow[]>(() => {
    if (isSearching) return []
    return [
      ...(media.data?.items ?? []).map((item) => ({
        key: `media-${item.id}`,
        to: `/media/${item.id}`,
        type: t('library.audio'),
        title: item.title || item.filename,
        status: item.scan_status || item.status,
        createdAt: item.created_at,
        icon: FileAudio,
      })),
      ...(transcriptions.data?.items ?? []).map((item) => ({
        key: `transcription-${item.id}`,
        to: `/transcriptions/${item.id}`,
        type: t('library.transcript'),
        title: item.media_title || item.media_filename || t('library.untitled'),
        status: item.status,
        createdAt: item.created_at,
        icon: FileText,
      })),
    ].sort((a, b) => b.createdAt.localeCompare(a.createdAt))
  }, [isSearching, media.data?.items, t, transcriptions.data?.items])

  const loading = isSearching
    ? transcriptSearch.isLoading
    : media.isLoading || transcriptions.isLoading
  const failed = isSearching ? transcriptSearch.isError : media.isError && transcriptions.isError
  const partialFailure = !isSearching && !failed && (media.isError || transcriptions.isError)
  const total = isSearching
    ? (transcriptSearch.data?.total ?? 0)
    : Math.max(media.data?.total ?? 0, transcriptions.data?.total ?? 0)
  const transcriptHasMoreCandidates =
    isSearching && transcriptSearch.data?.transcript_complete === false
  const transcriptCanContinue =
    transcriptHasMoreCandidates && Boolean(transcriptSearch.data?.next_transcript_cursor)
  const hasSearchResults =
    metadataRows.length > 0 || transcriptRows.length > 0 || transcriptHasMoreCandidates

  function retry() {
    if (isSearching) {
      transcriptSearch.refetch()
      return
    }
    media.refetch()
    transcriptions.refetch()
  }

  function updateSearch(value: string) {
    if (value.trim() === query) {
      setSearch(value)
      return
    }
    setSearch(value)
    setOffset(0)
    setTranscriptCursor(undefined)
    setTranscriptItems([])
    completedTranscriptPages.current.clear()
  }

  function loadMoreTranscriptMatches() {
    const cursor = transcriptSearch.data?.next_transcript_cursor
    if (cursor) setTranscriptCursor(cursor)
  }

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <header>
        <h1 className="text-3xl font-semibold">{t('library.title')}</h1>
        <p className="mt-2 text-muted-foreground">{t('library.description')}</p>
      </header>
      <label className="relative block max-w-xl">
        <span className="sr-only">{t('library.search')}</span>
        <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
        <Input
          value={search}
          onChange={(event) => updateSearch(event.target.value)}
          placeholder={t('library.search')}
          className="pl-9"
        />
      </label>
      {partialFailure && (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-destructive/40 p-4 text-destructive"
        >
          <span>{t('library.partialError')}</span>
          <Button type="button" variant="outline" size="sm" onClick={retry}>
            {t('library.retry')}
          </Button>
        </div>
      )}
      {failed ? (
        <ErrorState onRetry={retry} />
      ) : loading ? (
        <p className="text-muted-foreground">{t('library.loading')}</p>
      ) : isSearching && !hasSearchResults ? (
        <EmptyState search={isSearching} />
      ) : isSearching ? (
        <div className="space-y-6">
          {metadataRows.length > 0 && (
            <ResultList rows={metadataRows} label={t('library.metadataResults')} />
          )}
          {(transcriptRows.length > 0 || transcriptHasMoreCandidates) && (
            <section className="space-y-3" aria-label={t('library.transcriptMatches')}>
              <h2 className="text-lg font-semibold">{t('library.transcriptMatches')}</h2>
              {transcriptRows.length > 0 ? (
                <ResultList rows={transcriptRows} label={t('library.transcriptMatches')} />
              ) : (
                <p className="text-sm text-muted-foreground">{t('library.transcriptPageNoMatches')}</p>
              )}
              {transcriptHasMoreCandidates && (
                <div className="flex flex-wrap items-center gap-3">
                  <p className="text-sm text-muted-foreground">{t('library.transcriptSearchLimited')}</p>
                  {transcriptCanContinue && (
                    <Button type="button" variant="outline" onClick={loadMoreTranscriptMatches}>
                      {t('library.moreTranscriptMatches')}
                    </Button>
                  )}
                </div>
              )}
            </section>
          )}
        </div>
      ) : rows.length === 0 ? (
        <EmptyState search={false} />
      ) : (
        <ResultList rows={rows} label={t('library.title')} />
      )}
      <Pagination offset={offset} total={total} onPageChange={setOffset} />
    </div>
  )
}

function appendDistinctSearchItems(current: SearchItem[], next: SearchItem[]) {
  const known = new Set(current.map(transcriptResultKey))
  return [...current, ...next.filter((item) => !known.has(transcriptResultKey(item)))]
}

function transcriptResultKey(item: SearchItem) {
  return item.latest_transcription_id ?? item.id
}

function ResultList({ rows, label }: { rows: LibraryRow[]; label: string }) {
  return (
    <section className="overflow-hidden rounded-lg border bg-card" aria-label={label}>
      {rows.map((row) => {
        const Icon = row.icon
        return (
          <Link
            key={row.key}
            to={row.to}
            className="flex items-center gap-4 border-b p-4 last:border-b-0 hover:bg-accent"
          >
            <Icon className="h-5 w-5 text-muted-foreground" aria-hidden="true" />
            <span className="min-w-0 flex-1">
              <span className="block truncate font-medium">{row.title}</span>
              {row.detail && (
                <span className="block truncate text-sm text-muted-foreground">{row.detail}</span>
              )}
              <span className="block text-sm text-muted-foreground">
                {row.type} · {row.status}
              </span>
            </span>
            <span className="hidden text-xs text-muted-foreground sm:block">
              {new Date(row.createdAt).toLocaleDateString()}
            </span>
          </Link>
        )
      })}
    </section>
  )
}

function Pagination({
  offset,
  total,
  onPageChange,
}: {
  offset: number
  total: number
  onPageChange: (offset: number) => void
}) {
  const { t } = useTranslation('oss')
  if (total <= PAGE_SIZE) return null
  const currentPage = Math.floor(offset / PAGE_SIZE) + 1
  const totalPages = Math.ceil(total / PAGE_SIZE)
  return (
    <nav className="flex items-center justify-between" aria-label={t('library.pagination')}>
      <p className="text-sm text-muted-foreground">
        {t('library.page', { page: currentPage, pages: totalPages, limit: PAGE_SIZE })}
      </p>
      <span className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={offset === 0}
          onClick={() => onPageChange(Math.max(0, offset - PAGE_SIZE))}
          aria-label={t('library.previous')}
        >
          <ChevronLeft className="h-4 w-4" />
        </Button>
        <span className="text-sm text-muted-foreground">
          {currentPage} / {totalPages}
        </span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={offset + PAGE_SIZE >= total}
          onClick={() => onPageChange(offset + PAGE_SIZE)}
          aria-label={t('library.next')}
        >
          <ChevronRight className="h-4 w-4" />
        </Button>
      </span>
    </nav>
  )
}

function EmptyState({ search }: { search: boolean }) {
  const { t } = useTranslation('oss')
  if (search)
    return (
      <div className="rounded-lg border border-dashed p-8 text-center">
        <p className="font-medium">{t('library.noMatches')}</p>
      </div>
    )
  return (
    <div className="rounded-lg border border-dashed p-8 text-center">
      <p className="font-medium">{t('library.emptyTitle')}</p>
      <p className="mt-2 text-sm text-muted-foreground">{t('library.emptyBody')}</p>
      <Button asChild className="mt-4">
        <Link to="/dashboard?capture=upload">{t('library.upload')}</Link>
      </Button>
    </div>
  )
}

function ErrorState({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation('oss')
  return (
    <div className="rounded-md border border-destructive/40 p-4 text-destructive">
      <p>{t('library.error')}</p>
      <Button type="button" variant="outline" className="mt-3" onClick={onRetry}>
        {t('library.retry')}
      </Button>
    </div>
  )
}
