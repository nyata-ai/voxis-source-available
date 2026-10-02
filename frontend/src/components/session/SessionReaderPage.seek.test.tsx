import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SessionReaderPage } from './SessionReaderPage'
import type { SessionView } from './session-view'
import type { Utterance } from '@/types/transcription'

/**
 * The rest of the reader suite stubs the transcript and the player down to
 * their contracts, which leaves the one path nobody was covering: a reader
 * clicks a turn and the audio moves. This file mocks neither — the real
 * `TranscriptViewer` renders the turns, the real `AudioPlayer` owns the
 * element, and the assertions read the transport the way a reader would.
 */

const mockPreferences = vi.fn(() => ({ data: undefined as undefined }))
vi.mock('@/hooks/useSettings', () => ({
  usePreferences: () => mockPreferences(),
}))

vi.mock('@/hooks/useFeatures', () => ({
  useFeatures: () => ({ data: {} }),
}))

vi.mock('./BriefingPane', () => ({
  BriefingPane: () => <section id="briefing" data-testid="briefing-band" />,
}))

vi.mock('@/components/transcription/SpeakerEditor', () => ({
  SpeakerEditor: () => <div data-testid="speaker-editor" />,
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
    start: 42,
    end: 47,
    text: 'Thank you for having me.',
    confidence: 0.9,
    words: [],
  },
]

function makeSession(): SessionView {
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
    durationSeconds: 1796,
    wordCount: 10,
    speakerCount: 0,
    createdAt: '2026-08-01T10:00:00Z',
    utterances,
    audio: {
      streamUrl: 'https://example.com/audio.mp3',
      isLoading: false,
      state: 'available',
      onError: vi.fn(),
    },
    lenses: [],
    lensesReady: true,
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
      editTitle: false,
      editDescription: false,
      delete: false,
      export: false,
      downloadAudio: false,
      retranscribe: false,
      speakers: false,
      briefings: false,
    },
    actions: { saveTitle: vi.fn(), saveDescription: vi.fn() },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }
}

/** Gives the element a duration, which jsdom never reports, so the readout has
 *  a total to print beside the elapsed time. */
function primeDuration(container: HTMLElement, seconds: number) {
  const audio = container.querySelector('audio')
  if (!audio) throw new Error('no audio element rendered')
  Object.defineProperty(audio, 'duration', { value: seconds, configurable: true })
  Object.defineProperty(audio, 'currentTime', { value: 0, writable: true, configurable: true })
  act(() => {
    audio.dispatchEvent(new Event('loadedmetadata'))
  })
  return audio
}

/** An observer the test drives by hand, so the mini dock can be raised. */
function stubIntersectionObserver() {
  const state = { notify: null as ((intersecting: boolean) => void) | null }
  class FakeIntersectionObserver {
    constructor(callback: IntersectionObserverCallback) {
      state.notify = (isIntersecting) => {
        callback([{ isIntersecting } as IntersectionObserverEntry], this as never)
      }
    }
    observe() {}
    unobserve() {}
    disconnect() {
      state.notify = null
    }
  }
  vi.stubGlobal('IntersectionObserver', FakeIntersectionObserver)
  return state
}

function renderReader() {
  return render(
    <MemoryRouter initialEntries={['/sessions/tx-1']}>
      <SessionReaderPage session={makeSession()} />
    </MemoryRouter>,
  )
}

describe('SessionReaderPage — transcript and transport', () => {
  beforeEach(() => {
    // jsdom implements neither, and the real viewer auto-scrolls to the turn.
    Element.prototype.scrollIntoView = vi.fn()
    Element.prototype.scrollTo = vi.fn()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('moves the audio to the turn a reader clicks', async () => {
    const { container } = renderReader()
    const audio = primeDuration(container, 1796)
    const user = userEvent.setup()

    expect(screen.getByText('00:00 / 29:56')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Thank you for having me/ }))

    expect(audio.currentTime).toBe(42)
    expect(screen.getByText('00:42 / 29:56')).toBeInTheDocument()
  })

  // The card keeps its own time, so it would follow a click either way. The
  // page's copy — what the transcript highlight and this dock read — is the one
  // behind the 250 ms throttle, and a second click inside that window is
  // exactly the case the throttle used to swallow.
  it('reports a seek to the rest of the page even inside the throttle window', async () => {
    const observer = stubIntersectionObserver()
    const { container } = renderReader()
    primeDuration(container, 1796)
    act(() => { observer.notify?.(false) })
    const dock = screen.getByTestId('reader-mini-dock')
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: /Thank you for having me/ }))
    expect(within(dock).getByText('00:42 / 29:56')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Hello, welcome to the meeting/ }))
    expect(within(dock).getByText('00:00 / 29:56')).toBeInTheDocument()
  })

  it('moves the audio from the mini dock once the card has scrolled away', async () => {
    const observer = stubIntersectionObserver()
    const { container } = renderReader()
    const audio = primeDuration(container, 1000)

    act(() => { observer.notify?.(false) })
    const dock = screen.getByTestId('reader-mini-dock')

    const slider = screen.getAllByRole('slider').find((node) => dock.contains(node))
    if (!slider) throw new Error('no dock scrubber')
    slider.focus()
    await userEvent.setup().keyboard('{ArrowRight}')

    expect(audio.currentTime).toBeGreaterThan(0)
  })
})
