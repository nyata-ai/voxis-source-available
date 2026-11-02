import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { useSummariesByTranscription } from '@/hooks/useSummary'
import { ContinueCard } from './ContinueCard'
import type { LibraryItem } from './library'

vi.mock('@/hooks/useSummary', () => ({
  useSummariesByTranscription: vi.fn(() => ({ data: undefined })),
}))

function item(overrides: Partial<LibraryItem> = {}): LibraryItem {
  return {
    title: 'Site walk',
    to: '/transcriptions/tx-1',
    signature: 'tx-1',
    timestamp: 0,
    status: 'completed',
    kind: 'transcription',
    variant: 'audio',
    privilege: false,
    languages: [],
    ...overrides,
  } as LibraryItem
}

function renderCard(library: LibraryItem) {
  render(
    <MemoryRouter>
      <ContinueCard item={library} />
    </MemoryRouter>,
  )
}

describe('ContinueCard', () => {
  it('asks for the briefings of a regular session', () => {
    renderCard(item())
    expect(useSummariesByTranscription).toHaveBeenLastCalledWith('tx-1')
  })

  // The hook stays disabled on an empty id; the server would answer 404.
  it('does not ask for briefings on a privilege session', () => {
    renderCard(item({ privilege: true }))
    expect(useSummariesByTranscription).toHaveBeenLastCalledWith('')
    expect(screen.getByText('Site walk')).toBeInTheDocument()
  })
})
