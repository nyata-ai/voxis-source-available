import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/i18n'
import { toast } from '@/lib/toast'
import type { ActivityItem } from '@/types/activity'
import { ActivityCard } from './ActivityCard'

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn() },
}))

const startedAt = '2026-07-09T05:58:00Z'

const baseItem: ActivityItem = {
  kind: 'recording',
  ref_id: 'activity-1',
  title: 'Morning standup',
  stage: 'stitching',
  status: 'in_progress',
  started_at: startedAt,
}

function renderCard(item: ActivityItem, onDismiss = vi.fn()) {
  const queryClient = new QueryClient()
  const result = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <ActivityCard item={item} onDismiss={onDismiss} />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  )

  return { ...result, onDismiss }
}

describe('ActivityCard', () => {
  beforeEach(() => {
    vi.setSystemTime(new Date('2026-07-09T06:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('shows the step track, the current step and elapsed time for an in-progress recording', () => {
    vi.useFakeTimers()
    const { container } = renderCard(baseItem)

    expect(screen.getByText('Morning standup')).toBeInTheDocument()
    expect(screen.getByText('Saving')).toBeInTheDocument()
    expect(screen.getByText(/2m/)).toBeInTheDocument()
    // Saving → Ready, decorative, with the current step pulsing in place of a spinner.
    const track = screen.getByTestId('step-track')
    expect(track).toHaveAttribute('aria-hidden', 'true')
    expect([...track.querySelectorAll('[data-step]')].map((dot) => dot.getAttribute('data-step'))).toEqual([
      'current',
      'pending',
    ])
    expect(container.querySelector('[class*="animate-spin"]')).not.toBeInTheDocument()
    // Peach intake disc for a recording.
    expect(container.querySelector('[data-intake="record"]')).toBeInTheDocument()
  })

  it('uses inverse surface tokens and readable in-progress controls', () => {
    renderCard(baseItem)

    expect(screen.getByTestId('activity-card')).toHaveClass(
      'border-background/20',
      'bg-foreground',
      'text-background',
    )
    expect(screen.getByText('Saving').closest('div')).toHaveClass('text-background/70')
    expect(screen.getByRole('button', { name: 'Dismiss' })).toHaveClass(
      'text-background',
      'hover:bg-background/15',
    )
  })

  it('ticks elapsed time while an item remains in progress', () => {
    vi.useFakeTimers()
    renderCard({ ...baseItem, started_at: '2026-07-09T05:59:01Z' })

    expect(screen.getByText('59s')).toBeInTheDocument()

    act(() => {
      vi.advanceTimersByTime(1000)
    })

    expect(screen.getByText('1m')).toBeInTheDocument()
  })

  it('shows ready actions for a completed item', async () => {
    const user = userEvent.setup()
    const item = { ...baseItem, status: 'completed' as const, stage: 'ready' as const, link: '/media/media-1' }
    const { onDismiss } = renderCard(item)

    expect(screen.getByText('Ready')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'View' })).toHaveAttribute('href', '/media/media-1')

    await user.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(onDismiss).toHaveBeenCalledWith('activity-1')
  })

  it('shows failed copy without an inert retry action', () => {
    renderCard({
      ...baseItem,
      kind: 'transcription',
      status: 'failed',
      stage: 'transcribing',
    })

    expect(screen.getByText('Transcription failed')).toHaveClass('text-destructive-inverse')
    expect(screen.queryByRole('button', { name: 'Try again' })).not.toBeInTheDocument()
  })

  it('shows the stored failure reason with the full text on hover', () => {
    renderCard({
      ...baseItem,
      kind: 'transcription',
      status: 'failed',
      stage: 'transcribing',
      error_message: 'unsupported audio codec: aac',
    })

    const reason = screen.getByTestId('activity-error-message')
    expect(reason).toHaveTextContent('unsupported audio codec: aac')
    expect(reason).toHaveClass('line-clamp-2')
    expect(reason).toHaveAttribute('title', 'unsupported audio codec: aac')
  })

  it('does not show a failure reason while an item is still in progress', () => {
    renderCard({ ...baseItem, error_message: 'transient upload error' })

    expect(screen.queryByTestId('activity-error-message')).not.toBeInTheDocument()
  })

  it('offers View on a failed item so the session page can be opened', () => {
    renderCard({
      ...baseItem,
      kind: 'transcription',
      status: 'failed',
      stage: 'transcribing',
      link: '/transcriptions/trans-1',
      error_message: 'unsupported audio codec: aac',
    })

    expect(screen.getByRole('link', { name: 'View' })).toHaveAttribute(
      'href',
      '/transcriptions/trans-1',
    )
  })

  it('interpolates summary progress detail', () => {
    renderCard({
      ...baseItem,
      kind: 'transcription',
      stage: 'summarizing',
      detail: { terminal: 2, total: 4 },
    })

    expect(screen.getByText('Briefing 2 of 4')).toBeInTheDocument()
    expect(
      [...screen.getByTestId('step-track').querySelectorAll('[data-step]')].map((dot) =>
        dot.getAttribute('data-step'),
      ),
    ).toEqual(['done', 'current', 'pending'])
  })

  it('fires a ready toast once when an item becomes completed', () => {
    const { rerender } = renderCard(baseItem)
    expect(toast.success).not.toHaveBeenCalled()

    const completed = { ...baseItem, status: 'completed' as const, stage: 'ready' as const }
    rerender(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={new QueryClient()}>
          <MemoryRouter>
            <ActivityCard item={completed} onDismiss={vi.fn()} />
          </MemoryRouter>
        </QueryClientProvider>
      </I18nextProvider>,
    )
    rerender(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={new QueryClient()}>
          <MemoryRouter>
            <ActivityCard item={completed} onDismiss={vi.fn()} />
          </MemoryRouter>
        </QueryClientProvider>
      </I18nextProvider>,
    )

    expect(toast.success).toHaveBeenCalledOnce()
    expect(toast.success).toHaveBeenCalledWith('Recording ready')
  })

  it('does not fire a ready toast for an item that mounts already completed', () => {
    renderCard({ ...baseItem, status: 'completed', stage: 'ready' })

    expect(toast.success).not.toHaveBeenCalled()
  })

  it('allows dismissing an in-progress item', async () => {
    const user = userEvent.setup()
    const { onDismiss } = renderCard(baseItem)

    await user.click(screen.getByRole('button', { name: 'Dismiss' }))

    expect(onDismiss).toHaveBeenCalledWith('activity-1')
  })

  it('does not dismiss a failed card by itself', () => {
    vi.useFakeTimers()
    const onDismiss = vi.fn()
    renderCard({ ...baseItem, status: 'failed' }, onDismiss)

    act(() => {
      vi.advanceTimersByTime(60_000)
    })
    expect(onDismiss).not.toHaveBeenCalled()
  })

  it('gives View and Dismiss a 44 px target on phones', () => {
    renderCard({ ...baseItem, status: 'completed', stage: 'ready', link: '/media/1' })

    expect(screen.getByRole('link', { name: 'View' })).toHaveClass('h-11', 'md:h-8')
    expect(screen.getByRole('button', { name: 'Dismiss' })).toHaveClass('h-11', 'md:h-8')
  })

  it('auto-dismisses a completed card with its own timer', () => {
    vi.useFakeTimers()
    const onDismiss = vi.fn()
    renderCard({ ...baseItem, status: 'completed', stage: 'ready' }, onDismiss)

    act(() => {
      vi.advanceTimersByTime(9_999)
    })
    expect(onDismiss).not.toHaveBeenCalled()

    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(onDismiss).toHaveBeenCalledWith('activity-1')
  })
})
