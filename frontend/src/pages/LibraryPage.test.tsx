import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { LibraryPage } from './LibraryPage'

const useSearchMedia = vi.fn()
const useTranscriptionList = vi.fn()
const useUnifiedSearch = vi.fn()

vi.mock('@/hooks/useMedia', () => ({
  useSearchMedia: (...args: unknown[]) => useSearchMedia(...args),
}))
vi.mock('@/hooks/useTranscription', () => ({
  useTranscriptionList: (...args: unknown[]) => useTranscriptionList(...args),
}))
vi.mock('@/hooks/useSearch', () => ({
  useUnifiedSearch: (...args: unknown[]) => useUnifiedSearch(...args),
}))

const result = (data: unknown, overrides: Record<string, unknown> = {}) => ({
  data,
  isLoading: false,
  isError: false,
  refetch: vi.fn(),
  ...overrides,
})

function renderPage() {
  return render(
    <MemoryRouter>
      <LibraryPage />
    </MemoryRouter>
  )
}

const media = (id: number) => ({
  id: `media-${id}`,
  filename: `recording-${id}.mp3`,
  title: `Recording ${id}`,
  status: 'ready',
  scan_status: 'scan_clean',
  created_at: `2026-09-01T00:${String(id).padStart(2, '0')}:00Z`,
})

const searchItem = (id: number, overrides: Record<string, unknown> = {}) => ({
  id: `search-media-${id}`,
  entity_type: 'media',
  title: `Search result ${id}`,
  filename: `search-result-${id}.mp3`,
  status: 'completed',
  created_at: `2026-09-01T01:${String(id).padStart(2, '0')}:00Z`,
  latest_transcription_id: `search-tx-${id}`,
  score: 1,
  match_fields: ['title'],
  ...overrides,
})

const searchResponse = (query: string, overrides: Record<string, unknown> = {}) => ({
  items: [],
  total: 0,
  query,
  search_mode: 'lexical',
  semantic_available: false,
  transcript_items: [],
  transcript_complete: true,
  ...overrides,
})

describe('Voxis-OSS LibraryPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useSearchMedia.mockReturnValue(result({ items: [], total: 0 }))
    useTranscriptionList.mockReturnValue(result({ items: [], total: 0 }))
    useUnifiedSearch.mockReturnValue(result(searchResponse('')))
  })

  it('paginates beyond 50 records instead of fixing the library at its first page', async () => {
    useSearchMedia.mockReturnValue(
      result({ items: Array.from({ length: 50 }, (_, index) => media(index + 1)), total: 51 })
    )
    renderPage()
    const user = userEvent.setup()

    expect(screen.getByText('Page 1 of 2 of audio and metadata results.')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Next page' }))

    expect(useSearchMedia).toHaveBeenLastCalledWith({ limit: 50, offset: 50, enabled: true })
    expect(useTranscriptionList).toHaveBeenLastCalledWith(50, 50, '', { enabled: true })
  })

  it('uses the retained search endpoint for a transcript-only match and opens its transcript', async () => {
    useUnifiedSearch.mockReturnValue(
      result({
        ...searchResponse('audit-only phrase'),
        transcript_items: [
          {
            id: 'media-42',
            entity_type: 'media',
            title: 'Quarterly review',
            filename: 'review.mp3',
            status: 'completed',
            created_at: '2026-09-01T00:00:00Z',
            latest_transcription_id: 'tx-42',
            score: 1,
            match_fields: ['transcript'],
            snippet: 'The audit-only phrase appears in this transcript.',
          },
        ],
      })
    )
    renderPage()
    const user = userEvent.setup()

    await user.type(
      screen.getByRole('textbox', { name: 'Search audio and transcripts' }),
      'audit-only phrase'
    )

    expect(useUnifiedSearch).toHaveBeenLastCalledWith({
      search: 'audit-only phrase',
      limit: 50,
      offset: 0,
      transcriptCursor: undefined,
      enabled: true,
    })
    expect(
      await screen.findByText('The audit-only phrase appears in this transcript.')
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Quarterly review/ })).toHaveAttribute(
      'href',
      '/transcriptions/tx-42'
    )
  })

  it('continues a transcript search after an empty candidate page', async () => {
    useUnifiedSearch.mockImplementation((params: { search: string; transcriptCursor?: string }) => {
      if (params.transcriptCursor === 'cursor-after-empty') {
        return result(
          searchResponse(params.search, {
            transcript_items: [
              searchItem(99, {
                title: 'Later transcript match',
                snippet: 'The later completed transcript contains the searched phrase.',
                match_fields: ['transcript'],
              }),
            ],
          })
        )
      }

      return result(
        searchResponse(params.search, {
          items: [searchItem(1, { title: 'Metadata match' })],
          total: 1,
          next_transcript_cursor: 'cursor-after-empty',
          transcript_complete: false,
        })
      )
    })
    renderPage()
    const user = userEvent.setup()

    await user.type(screen.getByRole('textbox', { name: 'Search audio and transcripts' }), 'budget')

    expect(
      await screen.findByText(
        'No transcript matches in this page. More completed transcripts remain to search.'
      )
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Load more transcript matches' }))

    expect(useUnifiedSearch).toHaveBeenLastCalledWith({
      search: 'budget',
      limit: 50,
      offset: 0,
      transcriptCursor: 'cursor-after-empty',
      enabled: true,
    })
    expect(await screen.findByText('Later transcript match')).toBeInTheDocument()
  })

  it('keeps metadata pages and transcript matches independent beyond 50 results', async () => {
    useUnifiedSearch.mockImplementation((params: { search: string; offset: number }) =>
      result(
        searchResponse(params.search, {
          items:
            params.offset === 0
              ? Array.from({ length: 50 }, (_, index) => searchItem(index + 1))
              : [searchItem(51)],
          total: 51,
          transcript_items: [
            searchItem(75, {
              title: 'Transcript-content result',
              match_fields: ['transcript'],
            }),
          ],
        })
      )
    )
    renderPage()
    const user = userEvent.setup()

    await user.type(screen.getByRole('textbox', { name: 'Search audio and transcripts' }), 'budget')

    expect(await screen.findByText('Transcript-content result')).toBeInTheDocument()
    expect(screen.getByText('Page 1 of 2 of audio and metadata results.')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Next page' }))

    expect(useUnifiedSearch).toHaveBeenLastCalledWith({
      search: 'budget',
      limit: 50,
      offset: 50,
      transcriptCursor: undefined,
      enabled: true,
    })
    expect(await screen.findByText('Search result 51')).toBeInTheDocument()
  })

  it('resets the transcript cursor when the query changes', async () => {
    useUnifiedSearch.mockImplementation(
      (params: { search: string; transcriptCursor?: string }) =>
        result(
          searchResponse(params.search, {
            transcript_items:
              params.search === 'revised'
                ? [searchItem(2, { title: 'Revised content match', match_fields: ['transcript'] })]
                : [],
            next_transcript_cursor:
              params.search === 'budget' && !params.transcriptCursor ? 'budget-cursor' : undefined,
            transcript_complete: params.search !== 'budget' || Boolean(params.transcriptCursor),
          })
        )
    )
    renderPage()
    const user = userEvent.setup()
    const input = screen.getByRole('textbox', { name: 'Search audio and transcripts' })

    await user.type(input, 'budget')
    await user.click(await screen.findByRole('button', { name: 'Load more transcript matches' }))
    await user.clear(input)
    await user.type(input, 'revised')

    expect(useUnifiedSearch).toHaveBeenLastCalledWith({
      search: 'revised',
      limit: 50,
      offset: 0,
      transcriptCursor: undefined,
      enabled: true,
    })
    expect(await screen.findByText('Revised content match')).toBeInTheDocument()
  })

  it('keeps completed transcript matches when only whitespace changes', async () => {
    useUnifiedSearch.mockImplementation((params: { search: string }) =>
      result(
        searchResponse(params.search, {
          transcript_items: [
            searchItem(3, {
              title: 'Budget transcript match',
              match_fields: ['transcript'],
            }),
          ],
        })
      )
    )
    renderPage()
    const user = userEvent.setup()
    const input = screen.getByRole('textbox', { name: 'Search audio and transcripts' })

    await user.type(input, 'budget')
    expect(await screen.findByText('Budget transcript match')).toBeInTheDocument()

    await user.type(input, ' ')

    expect(screen.getByText('Budget transcript match')).toBeInTheDocument()
    expect(useUnifiedSearch).toHaveBeenLastCalledWith({
      search: 'budget',
      limit: 50,
      offset: 0,
      transcriptCursor: undefined,
      enabled: true,
    })
  })

  it('keeps distinct transcription matches for one media item across transcript pages', async () => {
    useUnifiedSearch.mockImplementation((params: { search: string; transcriptCursor?: string }) => {
      const sharedMedia = 'media-with-multiple-transcriptions'
      const firstPage = [
        searchItem(1, {
          id: sharedMedia,
          latest_transcription_id: 'transcription-first',
          title: 'First transcription match',
          match_fields: ['transcript'],
        }),
        searchItem(2, {
          id: sharedMedia,
          latest_transcription_id: 'transcription-second',
          title: 'Second transcription match',
          match_fields: ['transcript'],
        }),
      ]
      const nextPage = [
        searchItem(3, {
          id: sharedMedia,
          latest_transcription_id: 'transcription-third',
          title: 'Third transcription match',
          match_fields: ['transcript'],
        }),
      ]

      return result(
        searchResponse(params.search, {
          transcript_items: params.transcriptCursor ? nextPage : firstPage,
          next_transcript_cursor: params.transcriptCursor ? undefined : 'next-shared-media-page',
          transcript_complete: Boolean(params.transcriptCursor),
        })
      )
    })
    renderPage()
    const user = userEvent.setup()

    await user.type(screen.getByRole('textbox', { name: 'Search audio and transcripts' }), 'contract')

    expect(await screen.findByText('First transcription match')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Second transcription match/ })).toHaveAttribute(
      'href',
      '/transcriptions/transcription-second'
    )
    await user.click(screen.getByRole('button', { name: 'Load more transcript matches' }))

    expect(await screen.findByText('Third transcription match')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /First transcription match/ })).toHaveAttribute(
      'href',
      '/transcriptions/transcription-first'
    )
  })

  it('shows a search error instead of silently hiding a failed transcript request', async () => {
    useUnifiedSearch.mockReturnValue(result(undefined, { isError: true }))
    renderPage()
    const user = userEvent.setup()

    await user.type(screen.getByRole('textbox', { name: 'Search audio and transcripts' }), 'budget')

    expect(screen.getByText('The library could not be loaded. Try again shortly.')).toBeInTheDocument()
  })

  it('shows a partial-provider failure while retaining available rows', () => {
    useSearchMedia.mockReturnValue(result({ items: [media(1)], total: 1 }))
    useTranscriptionList.mockReturnValue(result(undefined, { isError: true }))
    renderPage()

    expect(screen.getByRole('alert')).toHaveTextContent('Part of this library could not be loaded.')
    expect(screen.getByText('Recording 1')).toBeInTheDocument()
  })
})
