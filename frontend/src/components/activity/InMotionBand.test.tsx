import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { InMotionBand } from './InMotionBand'
import { useActivity } from '@/hooks/useActivity'
import { mediaKeys } from '@/hooks/useMedia'
import { transcriptionKeys } from '@/hooks/useTranscription'
import type { ActivityItem } from '@/types/activity'

vi.mock('@/hooks/useActivity', () => ({
  useActivity: vi.fn(),
}))

function mockItems(items: ActivityItem[]) {
  vi.mocked(useActivity).mockReturnValue({
    data: { items },
  } as unknown as ReturnType<typeof useActivity>)
}

// The band invalidates the media list when an item leaves the in-flight set,
// so it needs a real client — and the rerenders below need the same one.
let queryClient = new QueryClient()

function wrap() {
  return (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <InMotionBand />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

function renderBand() {
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(wrap())
}

const startedAt = new Date(Date.now() - 90_000).toISOString()

describe('InMotionBand', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders no visible band when the activity feed is empty', () => {
    mockItems([])
    renderBand()
    expect(screen.queryByRole('heading')).not.toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })

  it('renders no visible band when the feed carries no in-flight items', () => {
    mockItems([
      {
        kind: 'transcription',
        ref_id: 't1',
        stage: 'ready',
        status: 'completed',
        started_at: startedAt,
      },
    ])
    renderBand()
    expect(screen.queryByRole('heading')).not.toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })

  it('renders no visible band while the activity feed is still loading', () => {
    vi.mocked(useActivity).mockReturnValue({
      data: undefined,
    } as unknown as ReturnType<typeof useActivity>)
    renderBand()
    expect(screen.queryByRole('heading')).not.toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })

  it('renders one line per in-flight item with its stage label and elapsed time', () => {
    mockItems([
      {
        kind: 'transcription',
        ref_id: 't1',
        title: 'Board Meeting',
        stage: 'transcribing',
        status: 'in_progress',
        link: '/transcriptions/t1',
        started_at: startedAt,
      },
      {
        kind: 'recording',
        ref_id: 'r1',
        stage: 'stitching',
        status: 'in_progress',
        started_at: startedAt,
      },
    ])
    renderBand()

    expect(screen.getByRole('heading', { name: /in motion/i })).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByText('Board Meeting')).toBeInTheDocument()
    // Stage vocabulary comes from activity-labels.ts — same strings the tray uses.
    expect(screen.getByText('Transcribing…')).toBeInTheDocument()
    expect(screen.getByText('Processing recording…')).toBeInTheDocument()
    // Untitled recordings fall back to the shared activity title.
    expect(screen.getByText('Live recording')).toBeInTheDocument()
    expect(screen.getAllByText('1m').length).toBeGreaterThanOrEqual(1)
  })

  it('renders the summarizing stage with its terminal/total counts', () => {
    mockItems([
      {
        kind: 'transcription',
        ref_id: 't1',
        title: 'Deposition',
        stage: 'summarizing',
        status: 'in_progress',
        detail: { terminal: 1, total: 3 },
        started_at: startedAt,
      },
    ])
    renderBand()
    expect(screen.getByText('Summarizing… 1 of 3')).toBeInTheDocument()
  })

  it('links a row to the item link when the feed provides one', () => {
    mockItems([
      {
        kind: 'transcription',
        ref_id: 't1',
        title: 'Board Meeting',
        stage: 'transcribing',
        status: 'in_progress',
        link: '/transcriptions/t1',
        started_at: startedAt,
      },
    ])
    renderBand()
    expect(screen.getByRole('link', { name: /board meeting/i })).toHaveAttribute(
      'href',
      '/transcriptions/t1'
    )
  })

  it('renders a link-less row as plain text', () => {
    mockItems([
      {
        kind: 'recording',
        ref_id: 'r1',
        stage: 'stitching',
        status: 'in_progress',
        started_at: startedAt,
      },
    ])
    renderBand()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.getByRole('listitem')).toBeInTheDocument()
  })

  it('renders a failed item in the destructive tone with its failure label', () => {
    mockItems([
      {
        kind: 'transcription',
        ref_id: 't1',
        title: 'Broken Upload',
        stage: 'transcribing',
        status: 'failed',
        started_at: startedAt,
      },
    ])
    renderBand()

    const label = screen.getByText('Transcription failed')
    expect(label).toBeInTheDocument()
    expect(label.className).toContain('text-destructive')
    // A failed item never shows an elapsed clock — it stopped moving.
    expect(screen.queryByText('1m')).not.toBeInTheDocument()
  })

  it('drops completed items but keeps the band alive for the rest', () => {
    mockItems([
      {
        kind: 'transcription',
        ref_id: 'done',
        title: 'Finished',
        stage: 'ready',
        status: 'completed',
        started_at: startedAt,
      },
      {
        kind: 'transcription',
        ref_id: 'live',
        title: 'Still Going',
        stage: 'transcribing',
        status: 'in_progress',
        started_at: startedAt,
      },
    ])
    renderBand()

    expect(screen.queryByText('Finished')).not.toBeInTheDocument()
    expect(screen.getByText('Still Going')).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(1)
  })
})

describe('InMotionBand live announcements', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  const runningItem: ActivityItem = {
    kind: 'transcription',
    ref_id: 't1',
    title: 'Board Meeting',
    stage: 'transcribing',
    status: 'in_progress',
    started_at: startedAt,
  }

  function announcementText() {
    return screen.getByRole('status').textContent
  }

  it('announces nothing on the initial render', () => {
    mockItems([runningItem])
    renderBand()
    expect(announcementText()).toBe('')
  })

  it('does not announce on a rerender with the same statuses (clock tick simulation)', () => {
    mockItems([runningItem])
    const { rerender } = renderBand()
    expect(announcementText()).toBe('')

    // A fresh poll response with identical statuses but a new object/array
    // identity — the same shape a per-second elapsed-time re-render would see.
    mockItems([{ ...runningItem }])
    rerender(wrap())
    expect(announcementText()).toBe('')
  })

  it('announces once when an item flips to failed', () => {
    mockItems([runningItem])
    const { rerender } = renderBand()
    expect(announcementText()).toBe('')

    mockItems([{ ...runningItem, status: 'failed' }])
    rerender(wrap())
    expect(announcementText()).toBe('Board Meeting: Transcription failed')
  })

  it('does not re-announce on a subsequent identical rerender', () => {
    mockItems([runningItem])
    const { rerender } = renderBand()

    mockItems([{ ...runningItem, status: 'failed' }])
    rerender(wrap())
    expect(announcementText()).toBe('Board Meeting: Transcription failed')

    mockItems([{ ...runningItem, status: 'failed' }])
    rerender(wrap())
    expect(announcementText()).toBe('Board Meeting: Transcription failed')
  })

  it('announces a title-qualified message when an item disappears (finishes)', () => {
    mockItems([runningItem])
    const { rerender } = renderBand()

    // The item completes: isInFlight drops it from the feed entirely.
    mockItems([])
    rerender(wrap())
    expect(announcementText()).toBe('Board Meeting: Ready')
  })

  it('refreshes cached media and transcription lists when an item settles', () => {
    mockItems([runningItem])
    const { rerender } = renderBand()
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')

    mockItems([])
    rerender(wrap())

    expect(invalidate).toHaveBeenCalledWith({ queryKey: mediaKeys.all })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: transcriptionKeys.all })
  })

  it('refreshes cached lists when an item changes to failed', () => {
    mockItems([runningItem])
    const { rerender } = renderBand()
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')

    mockItems([{ ...runningItem, status: 'failed' }])
    rerender(wrap())

    expect(invalidate).toHaveBeenCalledWith({ queryKey: mediaKeys.all })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: transcriptionKeys.all })
  })
})

describe('InMotionBand ticker', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
  })

  it('does not start the 1s ticker when no item is in_progress', () => {
    mockItems([
      {
        kind: 'transcription',
        ref_id: 't1',
        title: 'Broken Upload',
        stage: 'transcribing',
        status: 'failed',
        started_at: startedAt,
      },
    ])
    renderBand()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('starts the ticker while something runs and cleans it up on unmount', () => {
    mockItems([
      {
        kind: 'transcription',
        ref_id: 't1',
        title: 'Board Meeting',
        stage: 'transcribing',
        status: 'in_progress',
        started_at: startedAt,
      },
    ])
    const { unmount } = renderBand()
    expect(vi.getTimerCount()).toBeGreaterThan(0)

    unmount()
    expect(vi.getTimerCount()).toBe(0)
  })
})
