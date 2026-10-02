import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { SessionReaderPage } from './SessionReaderPage'
import type { SessionView } from './session-view'
import type { Utterance } from '@/types/transcription'

type PreferencesResult = {
  data:
    | { playback_speed: number; default_summary_type: string; summary_profile?: string }
    | undefined
  isLoading?: boolean
}
const mockPreferences = vi.fn<() => PreferencesResult>(() => ({ data: undefined }))
vi.mock('@/hooks/useSettings', () => ({
  usePreferences: () => mockPreferences(),
}))

vi.mock('@/hooks/useFeatures', () => ({
  useFeatures: () => ({ data: { summary_profiles: true } }),
}))

// The panes belong to the other half of this redesign (plan C1–C3). They are
// stubbed down to their contract here so this suite tests the shell — what it
// composes, what it passes, what it withholds — and not their internals.
vi.mock('@/components/transcription/TranscriptViewer', () => ({
  TranscriptViewer: ({ utterances }: { utterances: Utterance[] }) => (
    <div data-testid="transcript-scroll-container">{utterances.length} turns</div>
  ),
}))

vi.mock('@/components/transcription/SpeakerEditor', () => ({
  SpeakerEditor: () => <div data-testid="speaker-editor">Speakers</div>,
}))

vi.mock('@/components/media/AudioAnalysisCard', () => ({
  AudioAnalysisCard: () => <div data-testid="audio-analysis">Audio integrity</div>,
}))

vi.mock('./BriefingPane', () => ({
  BriefingPane: ({
    activeLens,
    onActiveLensChange,
  }: {
    activeLens?: string
    onActiveLensChange?: (type: string) => void
  }) => (
    <section id="briefing" data-testid="briefing-band" aria-label="Briefing">
      <span data-testid="briefing-active-lens">{activeLens}</span>
      <button type="button" onClick={() => onActiveLensChange?.('q_and_a')}>
        Pick Q&amp;A
      </button>
    </section>
  ),
}))

const utterances: Utterance[] = [
  {
    id: 1,
    speaker: 0,
    language: 'id',
    start: 0,
    end: 3.5,
    text: 'Hello, welcome to the meeting.',
    confidence: 0.95,
    words: [],
  },
  {
    id: 2,
    speaker: 1,
    language: 'id',
    start: 3.5,
    end: 7,
    text: 'Thank you for having me.',
    confidence: 0.9,
    words: [],
  },
]

function makeSession(overrides: Partial<SessionView> = {}): SessionView {
  return {
    source: 'media',
    id: 'tx-1',
    transcriptionId: 'tx-1',
    mediaId: 'media-1',
    status: 'completed',
    title: 'Board meeting',
    titlePlaceholder: 'board-meeting.mp3',
    description: 'Quarterly review',
    languages: ['id', 'en'],
    durationSeconds: 125,
    wordCount: 320,
    speakerCount: 2,
    createdAt: '2026-08-01T10:00:00Z',
    utterances,
    speakerMap: { '0': 'Alice', '1': 'Bob' },
    audio: {
      streamUrl: 'https://example.com/audio.mp3',
      isLoading: false,
      state: 'available',
      onError: vi.fn(),
    },
    lenses: [
      { type: 'general' },
      { type: 'key_points' },
      { type: 'action_items' },
      { type: 'q_and_a' },
    ],
    briefing: {
      generateType: vi.fn(),
      generateTypeError: null,
      generateAll: vi.fn(),
      isGeneratingAll: false,
      error: null,
      regenerate: vi.fn(),
      regenerateError: null,
    },
    can: {
      editTitle: true,
      editDescription: true,
      delete: true,
      export: true,
      downloadAudio: true,
      retranscribe: false,
      speakers: true,
      briefings: true,
    },
    actions: {
      saveTitle: vi.fn(),
      saveDescription: vi.fn(),
      delete: vi.fn(),
      isDeleting: false,
    },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
    ...overrides,
  }
}

const briefingBand = () => screen.getByTestId('briefing-band')
const queryBriefingBand = () => screen.queryByTestId('briefing-band')
const activeLens = () => screen.getByTestId('briefing-active-lens').textContent

/** Reads back what the page wrote to the URL. */
function SearchProbe() {
  const { search } = useLocation()
  return <span data-testid="search">{search}</span>
}

const search = () => screen.getByTestId('search').textContent

function reader(session: SessionView, entry = '/sessions/tx-1') {
  return (
    <MemoryRouter initialEntries={[entry]}>
      <SessionReaderPage session={session} />
      <SearchProbe />
    </MemoryRouter>
  )
}

function renderReader(session: SessionView, entry = '/sessions/tx-1') {
  return render(reader(session, entry))
}

describe('SessionReaderPage', () => {
  beforeEach(() => {
    mockPreferences.mockReturnValue({ data: undefined })
    Element.prototype.scrollTo = vi.fn()
    Element.prototype.scrollIntoView = vi.fn()
  })

  describe('shell states', () => {
    it('renders a loading state', () => {
      renderReader(makeSession({ isLoading: true }))
      expect(screen.getByRole('status')).toHaveTextContent('Loading transcription')
      expect(screen.queryByText('Board meeting')).not.toBeInTheDocument()
    })

    it('renders a not-found state with a retry', async () => {
      const session = makeSession({ isError: true })
      renderReader(session)

      expect(screen.getByText('Transcription not found or failed to load.')).toBeInTheDocument()

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Try again' }))
      expect(session.refetch).toHaveBeenCalled()
    })

    it('renders the processing state instead of the workspace', () => {
      renderReader(makeSession({ status: 'submitted', utterances: [] }))
      expect(screen.getByText('Processing transcription...')).toBeInTheDocument()
      expect(screen.queryByTestId('transcript-scroll-container')).not.toBeInTheDocument()
      expect(queryBriefingBand()).not.toBeInTheDocument()
      expect(screen.queryByTestId('reader-player')).not.toBeInTheDocument()
    })

    it('renders the failure message and keeps the header', () => {
      renderReader(
        makeSession({
          status: 'failed',
          errorMessage: 'Provider rejected the audio',
          utterances: [],
        })
      )
      expect(screen.getByText('Provider rejected the audio')).toBeInTheDocument()
      expect(screen.getByText('Board meeting')).toBeInTheDocument()
      expect(screen.queryByTestId('transcript-scroll-container')).not.toBeInTheDocument()
    })
  })

  describe('composition', () => {
    it('lays out header, workspace and briefing band', () => {
      renderReader(makeSession())

      expect(screen.getByTestId('reader-meta')).toBeInTheDocument()
      expect(screen.getByTestId('reader-player')).toBeInTheDocument()
      expect(screen.getByTestId('reader-export')).toBeInTheDocument()
      expect(screen.getByTestId('transcript-scroll-container')).toBeInTheDocument()
      expect(briefingBand()).toBeInTheDocument()
    })

    it('withholds the briefing band when briefings are not permitted', () => {
      const session = makeSession()
      renderReader({ ...session, can: { ...session.can, briefings: false } })

      expect(screen.getByTestId('transcript-scroll-container')).toBeInTheDocument()
      expect(queryBriefingBand()).not.toBeInTheDocument()
    })

    it('renders the speaker editor and the forensics card in the left rail', () => {
      renderReader(
        makeSession({
          forensics: { analysis: { checks: [], trust: 'high' } as never, fileHash: 'abc' },
        })
      )
      expect(screen.getByTestId('speaker-editor')).toBeInTheDocument()
      expect(screen.getByTestId('audio-analysis')).toBeInTheDocument()
    })
  })

  describe('audio', () => {
    it('renders the audio card when audio is available', () => {
      renderReader(makeSession())
      const card = screen.getByTestId('reader-player')
      expect(within(card).getByRole('button', { name: 'Play' })).toBeInTheDocument()
    })

    it('explains a pending scan instead of rendering a card', () => {
      const session = makeSession()
      renderReader({
        ...session,
        audio: { ...session.audio, streamUrl: null, state: 'scan_pending' },
      })

      expect(screen.queryByTestId('reader-player')).not.toBeInTheDocument()
      expect(screen.getByText(/File is being scanned for security/)).toBeInTheDocument()
    })

    it('explains deleted audio', () => {
      const session = makeSession()
      renderReader({ ...session, audio: { ...session.audio, streamUrl: null, state: 'deleted' } })
      expect(
        screen.getByText(/Audio deleted by live recording retention policy/)
      ).toBeInTheDocument()
    })

    it('offers a retry only for a failed stream', async () => {
      const retry = vi.fn()
      const session = makeSession()
      renderReader({
        ...session,
        audio: { ...session.audio, streamUrl: null, state: 'failed', retry },
      })

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Try again' }))
      expect(retry).toHaveBeenCalled()
    })

    // The dock is guarded on a real IntersectionObserver reporting the card out
    // of view. jsdom has none, so it must never appear here — a dock that
    // showed by default would cover the page on every render.
    it('never shows the mini dock without an IntersectionObserver', () => {
      renderReader(makeSession())
      expect(screen.queryByTestId('reader-mini-dock')).not.toBeInTheDocument()
    })
  })

  describe('briefing lens', () => {
    it('opens the general lens by default', () => {
      renderReader(makeSession())
      expect(activeLens()).toBe('general')
    })

    it('opens the lens a ?lens= deep link names', () => {
      renderReader(makeSession(), '/sessions/tx-1?lens=action_items')
      expect(activeLens()).toBe('action_items')
    })

    it('ignores a lens param that names no real briefing type', () => {
      renderReader(makeSession(), '/sessions/tx-1?lens=not_a_lens')
      expect(activeLens()).toBe('general')
    })

    it('opens the first ready lens when the URL names none', () => {
      renderReader(
        makeSession({
          lenses: [
            { type: 'general' },
            { type: 'key_points', summaryId: 's-2', status: 'completed' },
            { type: 'action_items' },
            { type: 'q_and_a' },
          ],
        })
      )
      expect(activeLens()).toBe('key_points')
    })

    it('falls back to the preferred lens when nothing is ready', () => {
      mockPreferences.mockReturnValue({
        data: { playback_speed: 1, default_summary_type: 'action_items' },
      })
      renderReader(makeSession())
      expect(activeLens()).toBe('action_items')
    })

    // Auto-briefings complete in whatever order the workers finish them. Once
    // the tab has been decided, one finishing later must not move it out from
    // under a reader who is already reading.
    it('does not move the open tab as later briefings complete', () => {
      const { rerender } = renderReader(makeSession())
      expect(activeLens()).toBe('general')

      rerender(
        reader(
          makeSession({
            lenses: [
              { type: 'general' },
              { type: 'key_points' },
              { type: 'action_items', summaryId: 's-3', status: 'completed' },
              { type: 'q_and_a' },
            ],
          })
        )
      )
      expect(activeLens()).toBe('general')
    })

    // The saved default and the briefing list are separate requests. Answering
    // as each lands walks the tab through two wrong answers on the way to the
    // right one, under a reader who has already started reading.
    it('waits for both the preference and the briefing list before it opens one', () => {
      mockPreferences.mockReturnValue({ data: undefined, isLoading: true })
      const { rerender } = renderReader(makeSession({ lensesReady: false }))
      expect(activeLens()).toBe('general')

      // The preference lands first; the briefing list still has not.
      mockPreferences.mockReturnValue({
        data: { playback_speed: 1, default_summary_type: 'action_items' },
      })
      rerender(reader(makeSession({ lensesReady: false })))
      expect(activeLens()).toBe('general')

      // Both settled: one answer, and the completed briefing outranks the
      // preference.
      rerender(
        reader(
          makeSession({
            lensesReady: true,
            lenses: [
              { type: 'general' },
              { type: 'key_points', summaryId: 's-2', status: 'completed' },
              { type: 'action_items' },
              { type: 'q_and_a' },
            ],
          })
        )
      )
      expect(activeLens()).toBe('key_points')

      // A preference that arrives (or refetches) afterwards cannot move it.
      mockPreferences.mockReturnValue({
        data: { playback_speed: 1, default_summary_type: 'q_and_a' },
      })
      rerender(reader(makeSession({ lensesReady: true })))
      expect(activeLens()).toBe('key_points')
    })

    it("keeps the reader's own tab when a later briefing completes", async () => {
      const { rerender } = renderReader(makeSession())
      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Pick Q&A' }))
      expect(activeLens()).toBe('q_and_a')

      rerender(
        reader(
          makeSession({
            lenses: [
              { type: 'general', summaryId: 's-1', status: 'completed' },
              { type: 'key_points' },
              { type: 'action_items' },
              { type: 'q_and_a' },
            ],
          })
        )
      )

      expect(activeLens()).toBe('q_and_a')
    })

    it("lets the pane's own tab choice win afterwards", async () => {
      renderReader(makeSession(), '/sessions/tx-1?lens=general')
      const user = userEvent.setup()

      await user.click(screen.getByRole('button', { name: 'Pick Q&A' }))

      expect(activeLens()).toBe('q_and_a')
    })

    it('scrolls the briefing band into view for a lens deep link', () => {
      renderReader(makeSession(), '/sessions/tx-1?lens=key_points')
      expect(Element.prototype.scrollIntoView).toHaveBeenCalled()
    })

    it('does not scroll when no lens was named', () => {
      renderReader(makeSession())
      expect(Element.prototype.scrollIntoView).not.toHaveBeenCalled()
    })
  })

  describe('the lens in the URL', () => {
    // Refresh and bookmark have to come back to the briefing being read.
    it('writes the open lens back to the URL when the reader picks a tab', async () => {
      renderReader(makeSession())
      const user = userEvent.setup()

      await user.click(screen.getByRole('button', { name: 'Pick Q&A' }))

      expect(search()).toBe('?lens=q_and_a')
    })

    it('writes the lens it resolved on its own', () => {
      renderReader(
        makeSession({
          lenses: [
            { type: 'general' },
            { type: 'key_points', summaryId: 's-2', status: 'completed' },
            { type: 'action_items' },
            { type: 'q_and_a' },
          ],
        })
      )
      expect(search()).toBe('?lens=key_points')
    })

    it('keeps the rest of the query string', async () => {
      renderReader(makeSession(), '/sessions/tx-1?tab=notes')
      const user = userEvent.setup()

      await user.click(screen.getByRole('button', { name: 'Pick Q&A' }))

      expect(search()).toBe('?tab=notes&lens=q_and_a')
    })

    // The write-back looks exactly like a deep link. If the scroll effect read
    // it as one, picking a tab would yank the page down to the briefing band.
    it('does not scroll to the band when it writes the lens itself', async () => {
      renderReader(makeSession())
      const user = userEvent.setup()

      await user.click(screen.getByRole('button', { name: 'Pick Q&A' }))

      expect(Element.prototype.scrollIntoView).not.toHaveBeenCalled()
    })
  })

  describe('a session without a workspace', () => {
    // The hash and the integrity checks describe the uploaded file, not the
    // transcript: a failed transcription is exactly when someone wants them.
    it('still shows audio integrity for a failed transcription', () => {
      renderReader(
        makeSession({
          status: 'failed',
          errorMessage: 'Provider rejected the audio',
          utterances: [],
          forensics: { analysis: { checks: [], trust: 'high' } as never, fileHash: 'abc' },
        })
      )

      expect(screen.getByText('Provider rejected the audio')).toBeInTheDocument()
      expect(screen.queryByTestId('transcript-scroll-container')).not.toBeInTheDocument()
      expect(screen.getByTestId('audio-analysis')).toBeInTheDocument()
    })

    it('still shows audio integrity while the transcription is running', () => {
      renderReader(
        makeSession({
          status: 'processing',
          utterances: [],
          forensics: { analysis: { checks: [], trust: 'high' } as never, fileHash: 'abc' },
        })
      )
      expect(screen.getByTestId('audio-analysis')).toBeInTheDocument()
    })

    it('shows nothing extra when the session has no forensics', () => {
      renderReader(makeSession({ status: 'failed', utterances: [], speakerCount: 0 }))
      expect(screen.queryByTestId('audio-analysis')).not.toBeInTheDocument()
      expect(screen.queryByTestId('speaker-editor')).not.toBeInTheDocument()
    })
  })

  describe('moving between sessions', () => {
    // Reader to reader reuses this component: every piece of reading state
    // belongs to the session it was set on.
    it('resets the find query when a different session arrives', async () => {
      const { rerender } = renderReader(makeSession())
      const user = userEvent.setup()

      const find = screen.getByRole('textbox', { name: 'Find in transcript' })
      await user.type(find, 'agenda')
      expect(find).toHaveValue('agenda')

      rerender(reader(makeSession({ id: 'tx-2', transcriptionId: 'tx-2' })))

      expect(screen.getByRole('textbox', { name: 'Find in transcript' })).toHaveValue('')
    })

    it('resets the follow switch when a different session arrives', async () => {
      const { rerender } = renderReader(makeSession())
      const user = userEvent.setup()

      await user.click(screen.getByRole('switch', { name: 'Follow playback' }))
      expect(screen.getByRole('switch', { name: 'Follow playback' })).not.toBeChecked()

      rerender(reader(makeSession({ id: 'tx-2', transcriptionId: 'tx-2' })))

      expect(screen.getByRole('switch', { name: 'Follow playback' })).toBeChecked()
    })
  })

  describe('dialogs', () => {
    it('opens the delete dialog from the More menu', async () => {
      const session = makeSession()
      renderReader(session)
      const user = userEvent.setup()

      await user.click(screen.getByRole('button', { name: 'More actions' }))
      await user.click(screen.getByRole('menuitem', { name: 'Delete' }))

      expect(screen.getByRole('alertdialog')).toBeInTheDocument()
    })
  })
})
