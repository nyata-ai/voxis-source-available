import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { SessionReaderPage } from './SessionReaderPage'
import type { SessionView } from './session-view'

const nonces: number[] = []

vi.mock('@/hooks/useSettings', () => ({
  usePreferences: () => ({ data: undefined }),
}))

vi.mock('@/hooks/useFeatures', () => ({
  useFeatures: () => ({ data: {} }),
}))

vi.mock('@/components/transcription/SpeakerEditor', () => ({
  SpeakerEditor: () => <div />,
}))

vi.mock('./BriefingPane', () => ({
  BriefingPane: ({ onCitationSeek }: { onCitationSeek?: (time: number) => void }) => (
    <button type="button" onClick={() => onCitationSeek?.(42)}>
      cite
    </button>
  ),
}))

vi.mock('@/components/transcription/TranscriptViewer', () => ({
  TranscriptViewer: ({ landing }: { landing?: { nonce: number } | null }) => {
    if (landing && !nonces.includes(landing.nonce)) nonces.push(landing.nonce)
    return <div />
  },
}))

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
    durationSeconds: 100,
    wordCount: 10,
    speakerCount: 0,
    createdAt: '2026-08-01T10:00:00Z',
    utterances: [],
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
      briefings: true,
    },
    actions: { saveTitle: vi.fn(), saveDescription: vi.fn() },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }
}

describe('SessionReaderPage: citations', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    nonces.length = 0
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  // The viewer scrolls once per nonce and remembers the last one. If a nonce came
  // back after the ring cleared, later citations would move the audio but leave
  // their turn off-screen.
  it('gives every citation a new nonce, even after the ring has cleared', () => {
    render(
      <MemoryRouter initialEntries={['/sessions/tx-1']}>
        <SessionReaderPage session={makeSession()} />
      </MemoryRouter>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'cite' }))
    act(() => {
      vi.advanceTimersByTime(5000)
    })
    fireEvent.click(screen.getByRole('button', { name: 'cite' }))

    expect(nonces).toHaveLength(2)
  })
})
