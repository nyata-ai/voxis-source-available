import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ExportPanel } from './ExportPanel'
import type { SessionView } from './session-view'

vi.mock('@/lib/api-client', () => {
  class ApiError extends Error {
    status: number
    constructor(status: number, message: string) {
      super(message)
      this.status = status
      this.name = 'ApiError'
    }
  }
  return { apiClient: { download: vi.fn() }, ApiError }
})

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { apiClient } from '@/lib/api-client'

function makeSession(overrides: Partial<SessionView> = {}): SessionView {
  return {
    source: 'media',
    id: 'tx-1',
    transcriptionId: 'tx-1',
    mediaId: 'media-1',
    status: 'completed',
    title: 'Board meeting',
    titlePlaceholder: 'board-meeting.mp3',
    description: '',
    languages: [],
    durationSeconds: 60,
    wordCount: 10,
    speakerCount: 2,
    createdAt: '2026-08-01T10:00:00Z',
    utterances: [],
    audio: { streamUrl: null, isLoading: false, state: 'unavailable', onError: vi.fn() },
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
    actions: { saveTitle: vi.fn(), saveDescription: vi.fn() },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
    ...overrides,
  }
}

function renderPanel(session: SessionView, activeLens: 'general' | 'q_and_a' = 'general', bapEligible = false) {
  return render(<ExportPanel session={session} activeLens={activeLens} bapEligible={bapEligible} />)
}

describe('ExportPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('rows', () => {
    it('lists transcript, briefing and audio', () => {
      renderPanel(makeSession())

      expect(screen.getByText('Transcript')).toBeInTheDocument()
      expect(screen.getByText('Briefing · General Summary')).toBeInTheDocument()
      expect(screen.getByText('Original audio')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Download Audio' })).toBeInTheDocument()
    })

    it('names the briefing row after the lens on screen', () => {
      renderPanel(makeSession(), 'q_and_a')
      expect(screen.getByText('Briefing · Questions & Answers')).toBeInTheDocument()
    })

    // `DownloadAudioButton` renders nothing until the row is completed, so a
    // label with no control beside it is all the row would be.
    it('drops the audio row until the media row is completed', () => {
      renderPanel(makeSession({ status: 'processing' }))

      expect(screen.queryByText('Original audio')).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'Download Audio' })).not.toBeInTheDocument()
    })

    it('drops the transcript row when export is not permitted', () => {
      const base = makeSession()
      renderPanel({ ...base, can: { ...base.can, export: false } })

      expect(screen.queryByText('Transcript')).not.toBeInTheDocument()
      expect(screen.getByText('Original audio')).toBeInTheDocument()
    })

    // `can.briefings` gates the briefing band; a session that may not read its
    // briefings must not be offered a row that exports them either.
    it('drops the briefing row when briefings are not permitted', () => {
      const base = makeSession()
      renderPanel({ ...base, can: { ...base.can, briefings: false } })

      expect(screen.queryByText(/^Briefing · /)).not.toBeInTheDocument()
      expect(screen.getByText('Transcript')).toBeInTheDocument()
    })

    it('renders nothing at all when only briefings were permitted and they are not', () => {
      const base = makeSession()
      const { container } = renderPanel({
        ...base,
        transcriptionId: undefined,
        can: { ...base.can, briefings: false, downloadAudio: false },
      })
      expect(container).toBeEmptyDOMElement()
    })

    it('renders nothing at all when neither export nor download is permitted', () => {
      const base = makeSession()
      const { container } = renderPanel({
        ...base,
        can: { ...base.can, export: false, downloadAudio: false },
      })
      expect(container).toBeEmptyDOMElement()
    })

    it('shows the Berita Acara row only when the session is eligible', () => {
      renderPanel(makeSession(), 'general', false)
      expect(screen.queryByText('Berita Acara draft')).not.toBeInTheDocument()

      renderPanel(makeSession(), 'general', true)
      expect(screen.getByText('Berita Acara draft')).toBeInTheDocument()
    })
  })

  describe('pills', () => {
    it('downloads the transcript in the format clicked', async () => {
      vi.mocked(apiClient.download).mockResolvedValueOnce(undefined)
      renderPanel(makeSession())
      const user = userEvent.setup()

      await user.click(screen.getByRole('button', { name: 'Transcript as DOCX' }))

      expect(apiClient.download).toHaveBeenCalledWith(
        '/transcriptions/tx-1/export?format=docx',
        'board-meeting.docx',
      )
    })

    it('offers all three transcript formats and no fourth', () => {
      renderPanel(makeSession())
      expect(screen.getByRole('button', { name: 'Transcript as PDF' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Transcript as DOCX' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Transcript as JSON' })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /as TXT/ })).not.toBeInTheDocument()
    })

    it('downloads the active briefing when it is ready', async () => {
      vi.mocked(apiClient.download).mockResolvedValueOnce(undefined)
      renderPanel(
        makeSession({
          lenses: [
            { type: 'general', summaryId: 'sum-9', status: 'completed' },
            { type: 'key_points' },
            { type: 'action_items' },
            { type: 'q_and_a' },
          ],
        }),
      )
      const user = userEvent.setup()

      await user.click(screen.getByRole('button', { name: 'Briefing · General Summary as PDF' }))

      expect(apiClient.download).toHaveBeenCalledWith(
        '/summaries/sum-9/export?format=pdf',
        'board-meeting.pdf',
      )
    })

    // `disabled` would make the pill unfocusable, and an unfocusable button
    // never surfaces its hint or its tooltip to a keyboard or screen-reader
    // user. It stays focusable and refuses the click instead.
    it('blocks the briefing pills without disabling them, and says why', async () => {
      renderPanel(makeSession())
      const pill = screen.getByRole('button', { name: 'Briefing · General Summary as PDF' })

      expect(pill).toBeEnabled()
      expect(pill).toHaveAttribute('aria-disabled', 'true')
      expect(pill).toHaveAccessibleDescription('Generate this briefing first to export it.')

      const user = userEvent.setup()
      await user.tab()
      await user.click(pill)

      expect(pill).toHaveFocus()
      expect(apiClient.download).not.toHaveBeenCalled()
    })

    it('blocks the briefing pills for a lens that only failed', () => {
      renderPanel(
        makeSession({
          lenses: [
            { type: 'general', summaryId: 'sum-9', status: 'failed' },
            { type: 'key_points' },
            { type: 'action_items' },
            { type: 'q_and_a' },
          ],
        }),
      )
      const pill = screen.getByRole('button', { name: 'Briefing · General Summary as JSON' })
      expect(pill).toHaveAttribute('aria-disabled', 'true')
      expect(pill).toHaveAccessibleDescription('Generate this briefing first to export it.')
    })

    // A ready briefing carries no hint at all — nothing to describe.
    it('leaves a ready briefing pill unblocked and undescribed', () => {
      renderPanel(
        makeSession({
          lenses: [
            { type: 'general', summaryId: 'sum-9', status: 'completed' },
            { type: 'key_points' },
            { type: 'action_items' },
            { type: 'q_and_a' },
          ],
        }),
      )
      const pill = screen.getByRole('button', { name: 'Briefing · General Summary as PDF' })

      expect(pill).not.toHaveAttribute('aria-disabled')
      expect(pill).toHaveAccessibleDescription('')
    })

    it('sends the Berita Acara draft to the BAP template', async () => {
      vi.mocked(apiClient.download).mockResolvedValueOnce(undefined)
      renderPanel(makeSession(), 'general', true)
      const user = userEvent.setup()

      await user.click(screen.getByRole('button', { name: 'Berita Acara draft as DOCX' }))

      expect(apiClient.download).toHaveBeenCalledWith(
        '/transcriptions/tx-1/export?format=docx&template=bap',
        'board-meeting-BAP-draf.docx',
      )
    })

    // Two blob downloads at once is not something a reader asked for.
    it('holds every pill while one export is in flight', async () => {
      let finish!: () => void
      vi.mocked(apiClient.download).mockReturnValueOnce(
        new Promise<void>((resolve) => { finish = resolve }),
      )
      renderPanel(makeSession())
      const user = userEvent.setup()

      await user.click(screen.getByRole('button', { name: 'Transcript as PDF' }))

      expect(screen.getByRole('button', { name: 'Transcript as PDF' })).toHaveAttribute(
        'aria-busy',
        'true',
      )
      expect(screen.getByRole('button', { name: 'Transcript as JSON' })).toBeDisabled()

      await act(async () => { finish() })
      expect(screen.getByRole('button', { name: 'Transcript as JSON' })).toBeEnabled()
    })
  })
})
