import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ReaderWorkspace } from './ReaderWorkspace'
import type { SessionView } from './session-view'
import type { Utterance } from '@/types/transcription'

// A stand-in for the viewer (plan C1): it reports matches the way the real one
// does and exposes the two contract props, so this suite tests the toolbar's
// wiring rather than the transcript's rendering.
vi.mock('@/components/transcription/TranscriptViewer', async () => {
  const { useEffect } = await import('react')
  return {
    TranscriptViewer: ({
      utterances,
      searchQuery,
      activeMatch,
      following,
      onMatchesChange,
      onFollowingChange,
    }: {
      utterances: Utterance[]
      searchQuery?: string
      activeMatch?: number
      following?: boolean
      onMatchesChange?: (matches: number[]) => void
      onFollowingChange?: (following: boolean) => void
    }) => {
      const needle = (searchQuery ?? '').trim().toLowerCase()
      const matches = needle
        ? utterances.flatMap((u, i) => (u.text.toLowerCase().includes(needle) ? [i] : []))
        : []
      const signature = matches.join(',')
      useEffect(() => {
        onMatchesChange?.(signature === '' ? [] : signature.split(',').map(Number))
      }, [signature, onMatchesChange])
      return (
        <div data-testid="transcript-scroll-container">
          <span data-testid="viewer-following">{String(following)}</span>
          <span data-testid="viewer-active-match">{activeMatch}</span>
          <button type="button" onClick={() => onFollowingChange?.(false)}>
            Release follow
          </button>
        </div>
      )
    },
  }
})

vi.mock('@/components/transcription/SpeakerEditor', () => ({
  SpeakerEditor: () => <div data-testid="speaker-editor">Speakers</div>,
}))

vi.mock('@/components/media/AudioAnalysisCard', () => ({
  AudioAnalysisCard: () => <div data-testid="audio-analysis">Audio integrity</div>,
}))

vi.mock('./ExportPanel', () => ({
  ExportPanel: ({ activeLens }: { activeLens: string }) => (
    <div data-testid="reader-export">{activeLens}</div>
  ),
}))

const utterances: Utterance[] = [
  {
    id: 1,
    speaker: 0,
    language: 'id',
    start: 0,
    end: 3,
    text: 'Hello, welcome to the meeting.',
    confidence: 0.9,
    words: [],
  },
  {
    id: 2,
    speaker: 1,
    language: 'id',
    start: 3,
    end: 6,
    text: 'Thank you for having me.',
    confidence: 0.9,
    words: [],
  },
  {
    id: 3,
    speaker: 0,
    language: 'id',
    start: 6,
    end: 9,
    text: 'Let us discuss the agenda.',
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
    description: '',
    languages: [],
    durationSeconds: 60,
    wordCount: 10,
    speakerCount: 2,
    createdAt: '2026-08-01T10:00:00Z',
    utterances,
    audio: { streamUrl: null, isLoading: false, state: 'unavailable', onError: vi.fn() },
    lenses: [{ type: 'general' }],
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

function renderWorkspace(session: SessionView, audioCard?: React.ReactNode) {
  return render(
    <ReaderWorkspace
      session={session}
      currentTime={0}
      onSeek={vi.fn()}
      activeLens="general"
      bapEligible={false}
      audioCard={audioCard}
    />
  )
}

const followSwitch = () => screen.getByRole('switch', { name: 'Follow playback' })
const findField = () => screen.getByRole('textbox', { name: 'Find in transcript' })

describe('ReaderWorkspace', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('left rail', () => {
    it('puts the audio card, export, speakers and provenance in order', () => {
      renderWorkspace(
        makeSession({
          forensics: { analysis: { checks: [], trust: 'high' } as never, fileHash: 'abc' },
        }),
        <div data-testid="audio-card">Player</div>
      )

      expect(screen.getByTestId('audio-card')).toBeInTheDocument()
      expect(screen.getByTestId('reader-export')).toHaveTextContent('general')
      expect(screen.getByTestId('speaker-editor')).toBeInTheDocument()
      expect(screen.getByTestId('audio-analysis')).toBeInTheDocument()
    })

    // The phone artboard reads: audio card, transcript, then Export, Speakers
    // and Integrity. In one column that is `order`, and the rail has to
    // dissolve for its two halves to sit either side of the transcript.
    it('puts the transcript directly under the audio card on a phone', () => {
      renderWorkspace(makeSession(), <div data-testid="audio-card">Player</div>)

      const audioBlock = screen.getByTestId('audio-card').parentElement
      expect(audioBlock).toHaveClass('order-1')
      expect(screen.getByTestId('transcript-scroll-container').closest('.order-2')).not.toBeNull()
      expect(screen.getByTestId('reader-export').closest('.order-3')).not.toBeNull()
      // Both halves of the rail dissolve so those three can interleave.
      expect(audioBlock?.parentElement).toHaveClass('contents')
      expect(audioBlock?.parentElement?.parentElement).toHaveClass('contents')
    })

    // The rail starts at the header's own height and carries the 16 px of air
    // as opaque padding, so scrolling content cannot show through the band
    // between the two sticky elements.
    // The rail flows with the page: no inner scroll region means no second
    // scrollbar appearing over the panels on hover.
    it('lets the rail flow with the page instead of scrolling inside itself', () => {
      renderWorkspace(makeSession(), <div data-testid="audio-card">Player</div>)

      const inner = screen.getByTestId('audio-card').parentElement?.parentElement
      const rail = inner?.parentElement

      expect(inner?.className).not.toMatch(/overflow-y-auto|max-h-/)
      expect(rail).not.toHaveClass('lg:sticky')
      expect(rail).toHaveClass('lg:pt-4')
    })

    // The taller of the two columns has no slack to stick within, so a sticky
    // class there only misleads. The transcript scrolls in its own box.
    it('pins the transcript column under the app header with the gap inside it', () => {
      renderWorkspace(makeSession(), <div data-testid="audio-card">Player</div>)
      const column = screen.getByTestId('transcript-scroll-container').closest('.order-2')
      expect(column).toHaveClass('lg:sticky')
      expect(column).toHaveClass('lg:top-[var(--app-header-h)]')
      expect(column).toHaveClass('lg:pt-4')
      expect(column).toHaveClass('lg:bg-background')
    })

    // The dock is fixed over the bottom 56 px of the viewport, which is the
    // bottom 56 px of the rail's scroll box too.
    it('shortens the rail while the mini dock is up', () => {
      const { container, rerender } = render(
        <ReaderWorkspace
          session={makeSession()}
          currentTime={0}
          onSeek={vi.fn()}
          activeLens="general"
          bapEligible={false}
          audioCard={<div data-testid="audio-card">Player</div>}
        />
      )
      const grid = container.firstElementChild as HTMLElement
      expect(grid.style.getPropertyValue('--reader-dock-h')).toBe('')

      rerender(
        <ReaderWorkspace
          session={makeSession()}
          currentTime={0}
          onSeek={vi.fn()}
          activeLens="general"
          bapEligible={false}
          audioCard={<div data-testid="audio-card">Player</div>}
          dockVisible
        />
      )
      expect(
        (container.firstElementChild as HTMLElement).style.getPropertyValue('--reader-dock-h')
      ).toBe('3.5rem')
    })

    it('withholds the audio block when a media card is unavailable', () => {
      renderWorkspace(makeSession())
      expect(screen.queryByTestId('reader-player')).not.toBeInTheDocument()
    })

    it('withholds the speaker editor when the capability is off', () => {
      const base = makeSession()
      renderWorkspace({ ...base, can: { ...base.can, speakers: false } })
      expect(screen.queryByTestId('speaker-editor')).not.toBeInTheDocument()
    })

    it('withholds the export panel when nothing may be exported', () => {
      const base = makeSession()
      renderWorkspace({ ...base, can: { ...base.can, export: false, downloadAudio: false } })
      expect(screen.queryByTestId('reader-export')).not.toBeInTheDocument()
    })
  })

  describe('follow playback', () => {
    it('starts on and passes the state down', () => {
      renderWorkspace(makeSession())
      expect(followSwitch()).toBeChecked()
      expect(screen.getByTestId('viewer-following')).toHaveTextContent('true')
    })

    it('releases follow from the switch', async () => {
      renderWorkspace(makeSession())
      const user = userEvent.setup()

      await user.click(followSwitch())

      expect(followSwitch()).not.toBeChecked()
      expect(screen.getByTestId('viewer-following')).toHaveTextContent('false')
    })

    it('follows the viewer when the reader scrolls away from playback', async () => {
      renderWorkspace(makeSession())
      const user = userEvent.setup()

      await user.click(screen.getByRole('button', { name: 'Release follow' }))

      expect(followSwitch()).not.toBeChecked()
    })

    it('is reachable by its label', async () => {
      renderWorkspace(makeSession())
      const user = userEvent.setup()

      await user.click(screen.getByText('Follow playback'))
      expect(followSwitch()).not.toBeChecked()
    })
  })

  describe('find in transcript', () => {
    it('reports the match count', async () => {
      renderWorkspace(makeSession())
      const user = userEvent.setup()

      await user.type(findField(), 'the')

      expect(screen.getByTestId('reader-find-count')).toHaveTextContent('1 of 2')
    })

    it('steps to the next match on Enter and wraps', async () => {
      renderWorkspace(makeSession())
      const user = userEvent.setup()

      await user.type(findField(), 'the')
      await user.type(findField(), '{Enter}')
      expect(screen.getByTestId('reader-find-count')).toHaveTextContent('2 of 2')
      expect(screen.getByTestId('viewer-active-match')).toHaveTextContent('1')

      await user.type(findField(), '{Enter}')
      expect(screen.getByTestId('reader-find-count')).toHaveTextContent('1 of 2')
    })

    it('steps backward on Shift+Enter', async () => {
      renderWorkspace(makeSession())
      const user = userEvent.setup()

      await user.type(findField(), 'the')
      await user.type(findField(), '{Shift>}{Enter}{/Shift}')

      expect(screen.getByTestId('reader-find-count')).toHaveTextContent('2 of 2')
    })

    it('clears the query on Escape', async () => {
      renderWorkspace(makeSession())
      const user = userEvent.setup()

      await user.type(findField(), 'the')
      await user.type(findField(), '{Escape}')

      expect(findField()).toHaveValue('')
      expect(screen.queryByTestId('reader-find-count')).not.toBeInTheDocument()
    })

    // ja/zh/ko input composes candidates in the field itself: Enter commits the
    // candidate and Escape cancels it. Acting on either would step or clear the
    // search out from under a user who was still typing a word.
    it('ignores Enter and Escape while an IME candidate is composing', async () => {
      renderWorkspace(makeSession())
      const user = userEvent.setup()

      await user.type(findField(), 'the')
      expect(screen.getByTestId('reader-find-count')).toHaveTextContent('1 of 2')

      fireEvent.keyDown(findField(), { key: 'Enter', isComposing: true })
      expect(screen.getByTestId('reader-find-count')).toHaveTextContent('1 of 2')

      fireEvent.keyDown(findField(), { key: 'Escape', isComposing: true })
      expect(findField()).toHaveValue('the')

      fireEvent.keyDown(findField(), { key: 'Enter' })
      expect(screen.getByTestId('reader-find-count')).toHaveTextContent('2 of 2')
    })

    it('reports no matches without disabling the field', async () => {
      renderWorkspace(makeSession())
      const user = userEvent.setup()

      await user.type(findField(), 'zzzz')

      expect(screen.getByTestId('reader-find-count')).toHaveTextContent('No matches')
      expect(screen.getByRole('button', { name: 'Next match' })).toBeDisabled()
    })

    it('clears the query from the clear button', async () => {
      renderWorkspace(makeSession())
      const user = userEvent.setup()

      await user.type(findField(), 'the')
      await user.click(screen.getByRole('button', { name: 'Clear search' }))

      expect(findField()).toHaveValue('')
    })
  })

  describe('transcript body', () => {
    it('renders the adapter call to action instead of the viewer', () => {
      renderWorkspace(
        makeSession({ utterances: [], transcriptCta: <button type="button">Transcribe</button> })
      )

      expect(screen.getByRole('button', { name: 'Transcribe' })).toBeInTheDocument()
      expect(screen.queryByTestId('transcript-scroll-container')).not.toBeInTheDocument()
    })

    it('hides the toolbar when there is no text to search or follow', () => {
      renderWorkspace(
        makeSession({ utterances: [], transcriptCta: <button type="button">Transcribe</button> })
      )

      expect(screen.queryByRole('textbox', { name: 'Find in transcript' })).not.toBeInTheDocument()
      expect(screen.queryByRole('switch')).not.toBeInTheDocument()
    })

    it('falls back to the plain transcript when there are no segments', () => {
      renderWorkspace(makeSession({ utterances: [], fullTranscript: 'A flat transcript.' }))
      expect(screen.getByText('A flat transcript.')).toBeInTheDocument()
    })
  })
})
