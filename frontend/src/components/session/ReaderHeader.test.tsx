import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ReaderHeader } from './ReaderHeader'
import type { SessionView } from './session-view'

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
    sizeBytes: 24_536_000,
    createdAt: '2026-08-01T10:00:00Z',
    utterances: [],
    audio: { streamUrl: null, isLoading: false, state: 'unavailable', onError: vi.fn() },
    lenses: [],
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

function renderHeader(
  session: SessionView,
  handlers: Partial<{ onDelete: () => void; onRetranscribe: () => void }> = {}
) {
  const onDelete = handlers.onDelete ?? vi.fn()
  const onRetranscribe = handlers.onRetranscribe ?? vi.fn()
  render(
    <MemoryRouter>
      <ReaderHeader session={session} onDelete={onDelete} onRetranscribe={onRetranscribe} />
    </MemoryRouter>
  )
  return { onDelete, onRetranscribe }
}

const meta = () => screen.getByTestId('reader-meta')

describe('ReaderHeader', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('chips', () => {
    it('shows the facts the session has', () => {
      renderHeader(
        makeSession({ encryptionAlgo: 'AES-256-GCM', preprocessorUsed: 'deepfilternet' })
      )

      expect(meta()).toHaveTextContent('02:05')
      expect(meta()).toHaveTextContent('2 speakers')
      expect(meta()).toHaveTextContent('320 words')
      expect(within(meta()).getByText('Encrypted')).toBeInTheDocument()
      expect(within(meta()).getByText('Enhanced')).toBeInTheDocument()
      expect(within(meta()).getByLabelText(/ID, EN/)).toBeInTheDocument()
    })

    it('omits every chip the session cannot fill', () => {
      renderHeader(
        makeSession({ durationSeconds: 0, wordCount: 0, speakerCount: 0, languages: [] })
      )

      expect(meta()).not.toHaveTextContent('speaker')
      expect(meta()).not.toHaveTextContent('words')
      expect(within(meta()).queryByText('Encrypted')).not.toBeInTheDocument()
      expect(within(meta()).queryByText('Enhanced')).not.toBeInTheDocument()
    })

    it('shows no Enhanced chip for an empty preprocessor string', () => {
      renderHeader(makeSession({ preprocessorUsed: '', encryptionAlgo: '' }))
      expect(within(meta()).queryByText('Enhanced')).not.toBeInTheDocument()
      expect(within(meta()).queryByText('Encrypted')).not.toBeInTheDocument()
    })

    // Finished work is not news; only live work and failures earn a status chip.
    it('carries a status chip only for a live or failed session', () => {
      renderHeader(makeSession({ status: 'completed' }))
      expect(within(meta()).queryByText('Completed')).not.toBeInTheDocument()

      screen.getByTestId('reader-meta').remove()
      renderHeader(makeSession({ status: 'processing' }))
      expect(within(meta()).getByText('Processing')).toBeInTheDocument()
    })
  })

  describe('source line', () => {
    it('reads uploaded date, size and format for a media session', () => {
      renderHeader(makeSession())
      expect(screen.getByText(/^Uploaded .*23\.4 MB · MP3$/)).toBeInTheDocument()
    })

    it('omits the format when the filename carries no extension', () => {
      renderHeader(makeSession({ titlePlaceholder: 'recording', sizeBytes: undefined }))
      const line = screen.getByText(/^Uploaded/)
      expect(line).not.toHaveTextContent('·')
    })
  })

  describe('more menu', () => {
    it('offers delete and retranscribe when both are permitted', async () => {
      const session = makeSession({
        can: { ...makeSession().can, retranscribe: true },
        retranscribeSource: { id: 'tx-1' } as never,
      })
      const { onDelete, onRetranscribe } = renderHeader(session)
      const user = userEvent.setup()

      await user.click(screen.getByRole('button', { name: 'More actions' }))
      expect(screen.getByRole('menuitem', { name: 'Retranscribe' })).toBeInTheDocument()
      await user.click(screen.getByRole('menuitem', { name: 'Delete' }))
      expect(onDelete).toHaveBeenCalled()

      await user.click(screen.getByRole('button', { name: 'More actions' }))
      await user.click(screen.getByRole('menuitem', { name: 'Retranscribe' }))
      expect(onRetranscribe).toHaveBeenCalled()
    })

    it('drops retranscribe when the session has no row to re-run', async () => {
      renderHeader(makeSession({ can: { ...makeSession().can, retranscribe: true } }))
      const user = userEvent.setup()

      await user.click(screen.getByRole('button', { name: 'More actions' }))
      expect(screen.queryByRole('menuitem', { name: 'Retranscribe' })).not.toBeInTheDocument()
    })

    it('hides the whole menu when neither action is permitted', () => {
      const base = makeSession()
      renderHeader({ ...base, can: { ...base.can, delete: false, retranscribe: false } })
      expect(screen.queryByRole('button', { name: 'More actions' })).not.toBeInTheDocument()
    })

    // Export and Download Audio moved to the export panel; a duplicate here
    // would be a second, differently-shaped way to do the same thing.
    it('carries neither export nor download', () => {
      renderHeader(makeSession())
      expect(screen.queryByRole('button', { name: 'Export' })).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /download/i })).not.toBeInTheDocument()
    })
  })

  describe('editing', () => {
    it('saves an inline title edit', async () => {
      const session = makeSession()
      renderHeader(session)
      const user = userEvent.setup()

      await user.click(screen.getByText('Board meeting'))
      const input = screen.getByRole('textbox', { name: 'Audio title' })
      await user.clear(input)
      await user.type(input, 'Renamed session{Enter}')

      expect(session.actions.saveTitle).toHaveBeenCalledWith('Renamed session')
    })

    it('renders title and description as plain text when editing is not permitted', () => {
      const base = makeSession()
      renderHeader({ ...base, can: { ...base.can, editTitle: false, editDescription: false } })

      expect(screen.getByText('Board meeting')).toBeInTheDocument()
      expect(screen.getByText('Quarterly review')).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'Audio title' })).not.toBeInTheDocument()
    })
  })

  it('links back to the library', () => {
    renderHeader(makeSession())
    expect(screen.getByRole('link', { name: 'Back to library' })).toHaveAttribute(
      'href',
      '/library'
    )
  })

  describe('description', () => {
    it('shows an existing description and keeps the menu free of an add item', async () => {
      renderHeader(makeSession())
      expect(screen.getByText('Quarterly review')).toBeInTheDocument()
      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'More actions' }))
      expect(screen.queryByRole('menuitem', { name: /Add a description/ })).not.toBeInTheDocument()
    })

    // An empty "Add a description" line cost a row of height on every visit;
    // the empty case now lives in the More menu and opens the editor directly.
    it('moves adding a description into the More menu and opens the editor from there', async () => {
      const session = makeSession({ description: '' })
      renderHeader(session)
      expect(screen.queryByText(/Add a description/)).not.toBeInTheDocument()

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'More actions' }))
      await user.click(screen.getByRole('menuitem', { name: /Add a description/ }))

      const editor = await screen.findByRole('textbox', { name: 'Audio description' })
      await user.type(editor, 'Site visit notes{Control>}{Enter}{/Control}')
      expect(session.actions.saveDescription).toHaveBeenCalledWith('Site visit notes')
    })

    it('offers the menu for adding a description even when no rare action is permitted', async () => {
      const base = makeSession({ description: '' })
      renderHeader({ ...base, can: { ...base.can, delete: false, retranscribe: false } })
      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'More actions' }))
      expect(screen.getAllByRole('menuitem')).toHaveLength(1)
    })
  })

  describe('section links', () => {
    const sections = [
      { id: 'transcript', labelKey: 'transcription:reader.sections.transcript' },
      { id: 'briefing', labelKey: 'transcription:reader.sections.briefings' },
      { id: 'export', labelKey: 'transcription:reader.sections.export' },
    ]

    it('draws the links in the sticky bar and scrolls to a section without a hash', async () => {
      const target = document.createElement('section')
      target.id = 'briefing'
      target.scrollIntoView = vi.fn()
      document.body.appendChild(target)
      render(
        <MemoryRouter>
          <ReaderHeader session={makeSession()} onDelete={vi.fn()} onRetranscribe={vi.fn()} sections={sections} />
        </MemoryRouter>,
      )
      const nav = screen.getByRole('navigation', { name: 'Sections' })
      expect(screen.getByTestId('reader-bar')).toContainElement(nav)
      expect(within(nav).getAllByRole('link').map((link) => link.textContent)).toEqual([
        'Transcript',
        'Briefings',
        'Export',
      ])

      const user = userEvent.setup()
      await user.click(within(nav).getByRole('link', { name: 'Briefings' }))
      expect(target.scrollIntoView).toHaveBeenCalledWith({ block: 'start', behavior: 'smooth' })
      expect(window.location.hash).toBe('')
      target.remove()
    })

    it('draws no links when there is only one place to go', () => {
      render(
        <MemoryRouter>
          <ReaderHeader
            session={makeSession()}
            onDelete={vi.fn()}
            onRetranscribe={vi.fn()}
            sections={sections.slice(0, 1)}
          />
        </MemoryRouter>,
      )
      expect(screen.queryByRole('navigation', { name: 'Sections' })).not.toBeInTheDocument()
      // The bar itself stays: it carries the way back.
      expect(screen.getByRole('link', { name: 'Back to library' })).toBeInTheDocument()
    })
  })

  describe('speaker chip', () => {
    it('leads the speaker count with one disc per speaker, up to five', () => {
      renderHeader(makeSession({ speakerCount: 7 }))
      const chip = within(meta()).getByText('7 speakers')
      expect(chip.querySelectorAll('[data-testid="speaker-disc"]')).toHaveLength(5)
    })
  })
})
